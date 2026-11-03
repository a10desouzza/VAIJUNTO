// TEC502 - Problema 2. Autor: Arthur de Andrade Souza.
// Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
package servidor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
	"vaijunto/internal/protocolo"
)

func TestCancelamentoAntesDaPartidaENotificacoes(t *testing.T) {
	c := criarCenario(t)
	r := c.reservar(t)
	c.agora = c.agora.Add(29*time.Minute + 59*time.Second)
	c.reautenticar(t)
	id := c.bc.Trechos[0].Trecho.ID
	if _, err := c.g.CancelarTrecho(c.m1, id); err == nil {
		t.Fatal("outro motorista cancelou")
	}
	for i := 0; i < 2; i++ {
		if _, err := c.g.CancelarTrecho(c.m2, id); err != nil {
			t.Fatal(err)
		}
	}
	if c.g.reservas[r.ID].Status != protocolo.Cancelada {
		t.Fatal("reserva não cancelada")
	}
	for _, a := range r.Assentos {
		if c.g.trechos[a.Trecho.ID].trecho.Assentos != 2 {
			t.Fatal("vaga não devolvida ou devolvida duas vezes")
		}
	}
	if c.g.caronas[c.bc.ID].status != protocolo.Parcial {
		t.Fatal("carona deveria ser parcial")
	}
	if c.g.trechos[c.bc.Trechos[1].Trecho.ID].trecho.Status != protocolo.Ativa {
		t.Fatal("trecho independente cancelado")
	}
	avisos, err := c.g.ConsultarNotificacoes(c.p)
	if err != nil || len(avisos) != 1 || avisos[0].ReservaID != r.ID || avisos[0].Lida {
		t.Fatalf("notificações: %+v %v", avisos, err)
	}
	if err := c.g.LerNotificacao(c.outro, avisos[0].ID); err == nil {
		t.Fatal("outro usuário leu aviso")
	}
	if err := c.g.LerNotificacao(c.p, avisos[0].ID); err != nil {
		t.Fatal(err)
	}
	avisos, _ = c.g.ConsultarNotificacoes(c.p)
	if !avisos[0].Lida {
		t.Fatal("aviso não marcado")
	}
	resultado, err := c.g.Buscar(c.outro, protocolo.BuscaItinerario{Origem: "C", Destino: "D", Data: "2099-10-01"})
	if err != nil || len(resultado.Itinerarios) != 1 {
		t.Fatalf("trecho não cancelado indisponível: %+v %v", resultado, err)
	}
}

func TestCancelamentoEConfirmacaoConcorrentes(t *testing.T) {
	c := criarCenario(t)
	inicio := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-inicio
			if i%2 == 0 {
				c.g.Confirmar(c.p, protocolo.ReservaItinerario{Chave: fmt.Sprint(i), TrechosIDs: []string{c.ab.Trechos[0].Trecho.ID, c.bc.Trechos[0].Trecho.ID}})
			} else {
				c.g.CancelarTrecho(c.m2, c.bc.Trechos[0].Trecho.ID)
			}
		}()
	}
	close(inicio)
	wg.Wait()
	for _, id := range []string{c.ab.Trechos[0].Trecho.ID, c.bc.Trechos[0].Trecho.ID} {
		if c.g.trechos[id].trecho.Assentos != 2 || len(c.g.trechos[id].ocupados) != 0 {
			t.Fatal("vagas inconsistentes após disputa")
		}
	}
	for _, r := range c.g.reservas {
		if r.Status != protocolo.Cancelada {
			t.Fatal("reserva ativa em trecho cancelado")
		}
	}
}

func TestSocketNDJSON(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancelar := context.WithCancel(context.Background())
	fim := make(chan error, 1)
	go func() {
		fim <- Servir(ctx, listener, NovoGrafo(), time.Second)
	}()
	defer func() {
		cancelar()
		if err := <-fim; err != nil {
			t.Error(err)
		}
	}()
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	for _, parte := range []string{`{"acao":"CADAS`, "TRAR\",\"dados\":{\"nome\":\"Pessoa\",\"email\":\"teste@exemplo.com\",\"senha\":\"senha12345\",\"perfil\":\"PASSAGEIRO\"}}\n{invalido}\n"} {
		if _, err := conn.Write([]byte(parte)); err != nil {
			t.Fatal(err)
		}
	}
	reader := bufio.NewReader(conn)

	for _, status := range []string{protocolo.Sucesso, protocolo.Erro} {
		linha, err := reader.ReadBytes('\n')
		if err != nil {
			t.Fatal(err)
		}
		var r protocolo.Resposta
		if err := json.Unmarshal(linha, &r); err != nil {
			t.Fatal(err)
		}
		if r.Status != status || r.Mensagem == "" {
			t.Fatalf("resposta: %s", linha)
		}
	}
}

func TestConexaoOciosaNaoBloqueiaFilaDeRequisicoes(t *testing.T) {
	g := NovoGrafo()
	// O prazo mede atendimento de rede, sem incluir o custo do hash de senha.
	if _, err := g.Cadastrar(protocolo.Cadastro{Nome: "Pessoa Teste", Email: "fila@teste.com", Senha: "senha1234", Perfil: protocolo.Passageiro}); err != nil {
		t.Fatal(err)
	}
	sessao, err := g.Autenticar(protocolo.Credenciais{Email: "fila@teste.com", Senha: "senha1234"})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancelar := context.WithCancel(context.Background())
	fim := make(chan error, 1)
	go func() {
		fim <- Servir(ctx, listener, g, 5*time.Second)
	}()
	defer func() {
		cancelar()
		if err := <-fim; err != nil {
			t.Error(err)
		}
	}()
	ociosa, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer ociosa.Close()
	ativa, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer ativa.Close()
	ativa.SetDeadline(time.Now().Add(time.Second))
	if err := json.NewEncoder(ativa).Encode(protocolo.Requisicao{Acao: protocolo.AcaoReservas, SessaoID: sessao.ID, Dados: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	linha, err := bufio.NewReader(ativa).ReadBytes('\n')
	if err != nil || !bytes.Contains(linha, []byte(`"status":"SUCESSO"`)) {
		t.Fatalf("conexao ociosa bloqueou outra requisicao: %s %v", linha, err)
	}
}

func TestAtenderConexaoIgnoraLinhaIncompleta(t *testing.T) {
	g := NovoGrafo()
	servidor, cliente := net.Pipe()
	concluido := make(chan struct{})
	go func() {
		AtenderConexao(servidor, g, time.Second)
		close(concluido)
	}()

	io.WriteString(cliente, `{"acao":"CADASTRAR","dados":{"nome":"Ana Teste","email":"ana@teste.com","senha":"senha1234","perfil":"PASSAGEIRO"}}`)
	cliente.Close()
	<-concluido
	if len(g.usuarios) != 0 {
		t.Fatal("requisição sem quebra de linha alterou o estado")
	}
}

func TestAtenderConexaoRejeitaLinhaAcimaDoLimite(t *testing.T) {
	servidor, cliente := net.Pipe()
	concluido := make(chan struct{})
	go func() {
		AtenderConexao(servidor, NovoGrafo(), time.Second)
		close(concluido)
	}()

	escrita := make(chan error, 1)
	go func() {
		_, err := cliente.Write(append(bytes.Repeat([]byte{'x'}, protocolo.LimiteMensagem+1), '\n'))
		escrita <- err
	}()
	linha, err := bufio.NewReader(cliente).ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var resposta protocolo.Resposta
	if err := json.Unmarshal(linha, &resposta); err != nil {
		t.Fatal(err)
	}
	if resposta.Status != protocolo.Erro || !strings.Contains(resposta.Mensagem, "limite") {
		t.Fatalf("resposta = %+v", resposta)
	}
	cliente.Close()
	<-escrita
	<-concluido
}
