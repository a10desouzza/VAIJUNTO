// Arquivo: testes/integracao_test.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Exercita o servidor pela interface pública e pelo transporte TCP.
// Reúne os cenários locais de busca, atomicidade, concorrência e resposta perdida.
// Inclui cancelamento, proteção das cópias e carga; a coordenação remota é testada no servidor.

package testes

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"vaijunto/internal/carga"
	"vaijunto/internal/protocolo"
	"vaijunto/internal/servidor"
)

type ambiente struct {
	g               *servidor.GrafoItinerarios
	motorista       string
	outroMotorista  string
	passageiro      string
	outroPassageiro string
}

// usuario: Cadastra uma conta de teste e autentica para que os cenarios possam chamar operacoes
// protegidas. Retorna: Token autenticado. Interrompe o teste com Fatal se nao conseguir preparar a
// conta.
// Recebe: t testing.TB, g *servidor.GrafoItinerarios, email, perfil string.
// Retorna: string.
func usuario(t testing.TB, g *servidor.GrafoItinerarios, email, perfil string) string {
	t.Helper()
	if _, err := g.Cadastrar(protocolo.Cadastro{Nome: "Pessoa Teste", Email: email, Senha: "senha-teste-123", Perfil: perfil}); err != nil {
		t.Fatal(err)
	}
	s, err := g.Autenticar(protocolo.Credenciais{Email: email, Senha: "senha-teste-123"})
	if err != nil {
		t.Fatal(err)
	}
	return s.ID
}

// preparar: Isola os dados de cada teste e prepara contas com perfis diferentes. Retorna: Ambiente
// com grafo novo e sessoes de dois motoristas e dois passageiros.
// Recebe: t testing.TB.
// Retorna: ambiente.
func preparar(t testing.TB) ambiente {
	t.Helper()
	g := servidor.NovoGrafo()
	return ambiente{g: g, motorista: usuario(t, g, "m@teste.com", protocolo.Motorista), outroMotorista: usuario(t, g, "m2@teste.com", protocolo.Motorista), passageiro: usuario(t, g, "p@teste.com", protocolo.Passageiro), outroPassageiro: usuario(t, g, "p2@teste.com", protocolo.Passageiro)}
}

// oferta: Monta dados padrao de distancia e tempos para reutilizar nos cenarios. Retorna:
// PublicacaoCarona com uma oferta por par de cidades consecutivas.
// Recebe: chave string, rota []string, hora string, vagas int.
// Retorna: protocolo.PublicacaoCarona.
func oferta(chave string, rota []string, hora string, vagas int) protocolo.PublicacaoCarona {
	trechos := make([]protocolo.OfertaTrecho, len(rota)-1)
	for i := range trechos {
		trechos[i] = protocolo.OfertaTrecho{Preco: 10, DistanciaKM: 10, TempoViagem: 60, TempoParada: 15}
	}
	return protocolo.PublicacaoCarona{Chave: chave, Rota: rota, DataHora: hora, Assentos: vagas, Trechos: trechos}
}

// publicar: Usa a funcao oferta e publica no estado central do cenario. Retorna: Carona publicada.
// Interrompe o teste com Fatal se a preparacao falhar.
// Recebe: t testing.TB, a ambiente, sessaoID, chave string, rota []string, hora string, vagas int.
// Retorna: protocolo.Carona.
func publicar(t testing.TB, a ambiente, sessaoID, chave string, rota []string, hora string, vagas int) protocolo.Carona {
	t.Helper()
	c, err := a.g.Publicar(sessaoID, oferta(chave, rota, hora, vagas))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// ids: Extrai os IDs para montar uma requisicao de reserva. Retorna: Lista com IDs de seus trechos
// na mesma ordem.
// Recebe: c protocolo.Carona.
// Retorna: []string.
func ids(c protocolo.Carona) []string {
	r := make([]string, len(c.Trechos))
	for i, t := range c.Trechos {
		r[i] = t.Trecho.ID
	}
	return r
}

// TestMultigrafoDataConexoesEOrdenacao: Verifica ofertas paralelas, filtro por data,
// compatibilidade das conexoes e ordenacao dos itinerarios.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestMultigrafoDataConexoesEOrdenacao(t *testing.T) {
	a := preparar(t)
	publicar(t, a, a.motorista, "ab", []string{"A", "B"}, "2099-10-01T08:00:00-03:00", 2)
	publicar(t, a, a.outroMotorista, "bc", []string{"B", "C"}, "2099-10-01T09:15:00-03:00", 2)
	publicar(t, a, a.motorista, "ab2", []string{"A", "B"}, "2099-10-01T08:00:00-03:00", 2)
	publicar(t, a, a.outroMotorista, "cedo", []string{"B", "C"}, "2099-10-01T08:59:00-03:00", 2)
	publicar(t, a, a.motorista, "amanha", []string{"A", "C"}, "2099-10-02T08:00:00-03:00", 2)
	publicar(t, a, a.outroMotorista, "ciclo", []string{"B", "A"}, "2099-10-01T09:15:00-03:00", 2)
	direto := oferta("direto", []string{"A", "C"}, "2099-10-01T08:00:00-03:00", 2)
	direto.Trechos[0].Preco = 30
	if _, err := a.g.Publicar(a.motorista, direto); err != nil {
		t.Fatal(err)
	}
	r, err := a.g.Buscar(a.passageiro, protocolo.BuscaItinerario{Origem: " a ", Destino: "C", Data: "2099-10-01"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Itinerarios) != 3 {
		t.Fatalf("caminhos: %+v", r)
	}
	// Partidas iguais: o preco e o primeiro desempate.
	if r.Itinerarios[0].PrecoTotal != 20 || r.Itinerarios[0].DuracaoTotal != 135 {
		t.Fatalf("pesos: %+v", r.Itinerarios[0])
	}
}

// TestRoteadorRejeitaMensagensInvalidas: Envia mensagens malformadas ou incompativeis e verifica
// respostas de erro do roteador.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestRoteadorRejeitaMensagensInvalidas(t *testing.T) {
	g := servidor.NovoGrafo()
	for _, msg := range []string{`{`, `null`, `[]`, `{}`, `{"acao":"X","dados":{}}`, `{"acao":"CADASTRAR","dados":null}`, `{"acao":"AUTENTICAR","dados":{"email":"x","extra":1}}`, `{"acao":"AUTENTICAR","acao":"CADASTRAR","dados":{}}`, `{"acao":"CADASTRAR","dados":{"email":"a","email":"b"}}`, `{"acao":"DESCONECTAR","dados":{}}`} {
		raw := servidor.ProcessarMensagem([]byte(msg), g)
		var r protocolo.Resposta
		if err := json.Unmarshal(raw, &r); err != nil {
			t.Fatal(err)
		}
		if r.Status != protocolo.Erro || r.Mensagem == "" || raw[len(raw)-1] != '\n' {
			t.Fatalf("resposta inválida: %s", raw)
		}
	}
}

// TestPublicacaoInvalidaNaoCriaTrechos: Tenta publicar dados invalidos e verifica que nao foram
// criados trechos parciais.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestPublicacaoInvalidaNaoCriaTrechos(t *testing.T) {
	a := preparar(t)
	for _, alterar := range []func(*protocolo.PublicacaoCarona){
		func(p *protocolo.PublicacaoCarona) {
			p.Rota = []string{"A", "A"}
		},
		func(p *protocolo.PublicacaoCarona) {
			p.Assentos = 0
		},
		func(p *protocolo.PublicacaoCarona) {
			p.Trechos[0].DistanciaKM = -1
		},
		func(p *protocolo.PublicacaoCarona) {
			p.Trechos[0].DistanciaKM = 0.001
		},
		func(p *protocolo.PublicacaoCarona) {
			p.DataHora = "amanhã"
		},
		func(p *protocolo.PublicacaoCarona) {
			p.Trechos = nil
		},
	} {
		p := oferta("x", []string{"A", "B"}, "2099-10-01T08:00:00-03:00", 2)
		alterar(&p)
		if _, err := a.g.Publicar(a.motorista, p); err == nil {
			t.Fatal("publicação inválida aceita")
		}
	}
	caronas, _ := a.g.ConsultarCaronas(a.motorista)
	if len(caronas) != 0 {
		t.Fatal("publicação parcial")
	}
}

// TestCopiasNaoExpoemEstado: Altera dados devolvidos nas respostas e verifica que as copias nao
// permitem modificar o estado central.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestCopiasNaoExpoemEstado(t *testing.T) {
	a := preparar(t)
	c := publicar(t, a, a.motorista, "ab", []string{"A", "B"}, "2099-10-01T08:00:00-03:00", 2)
	r, err := a.g.Confirmar(a.passageiro, protocolo.ReservaItinerario{Chave: "x", TrechosIDs: ids(c)})
	if err != nil {
		t.Fatal(err)
	}
	r.Assentos[0].Numero = 99
	c.Rota[0] = "ALTERADA"
	reservas, _ := a.g.ConsultarReservas(a.passageiro)
	caronas, _ := a.g.ConsultarCaronas(a.motorista)
	if reservas[0].Assentos[0].Numero != 1 || caronas[0].Rota[0] != "A" {
		t.Fatal("estado interno exposto")
	}
}

// TestConcorrenciaUltimoAssento: Disputa a ultima vaga com goroutines e confere que apenas uma
// reserva completa foi confirmada.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestConcorrenciaUltimoAssento(t *testing.T) {
	a := preparar(t)
	c := publicar(t, a, a.motorista, "abc", []string{"A", "B", "C"}, "2099-10-01T08:00:00-03:00", 1)
	var sucessos atomic.Int32
	var wg sync.WaitGroup
	inicio := make(chan struct{})
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-inicio
			sessaoID := a.passageiro
			if i%2 == 1 {
				sessaoID = a.outroPassageiro
			}
			if _, err := a.g.Confirmar(sessaoID, protocolo.ReservaItinerario{Chave: fmt.Sprint(i), TrechosIDs: ids(c)}); err == nil {
				sucessos.Add(1)
			}
		}()
	}
	close(inicio)
	wg.Wait()
	if sucessos.Load() != 1 {
		t.Fatalf("reservas confirmadas: %d", sucessos.Load())
	}
	caronas, err := a.g.ConsultarCaronas(a.motorista)
	if err != nil {
		t.Fatal(err)
	}
	for _, trecho := range caronas[0].Trechos {
		if trecho.Trecho.Assentos != 0 || len(trecho.Passageiros) != 1 || trecho.Passageiros[0].Assento != 1 {
			t.Fatalf("overbooking: %+v", trecho)
		}
	}
}

// TestAtomicidadeEAssentoPorTrecho: Confere que uma falha em um trecho nao ocupa os demais e que a
// disponibilidade e controlada por trecho.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestAtomicidadeEAssentoPorTrecho(t *testing.T) {
	a := preparar(t)
	c := publicar(t, a, a.motorista, "abc", []string{"A", "B", "C"}, "2099-10-01T08:00:00-03:00", 1)
	caminho := ids(c)
	for i, invalida := range [][]string{nil, {caminho[0], caminho[0]}, {caminho[0], "inexistente"}, {caminho[1], caminho[0]}} {
		if _, err := a.g.Confirmar(a.passageiro, protocolo.ReservaItinerario{Chave: fmt.Sprint(i), TrechosIDs: invalida}); err == nil {
			t.Fatalf("aceitou %v", invalida)
		}
	}
	if _, err := a.g.Confirmar(a.outroPassageiro, protocolo.ReservaItinerario{Chave: "bc", TrechosIDs: caminho[1:]}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.g.Confirmar(a.passageiro, protocolo.ReservaItinerario{Chave: "abc", TrechosIDs: caminho}); err == nil {
		t.Fatal("reserva parcial aceita")
	}
	if _, err := a.g.Confirmar(a.passageiro, protocolo.ReservaItinerario{Chave: "ab", TrechosIDs: caminho[:1]}); err != nil {
		t.Fatalf("primeiro trecho indevidamente bloqueado: %v", err)
	}
}

// TestIdempotenciaECancelamento: Repete publicacoes, confirmacoes e cancelamentos para verificar
// que nao duplicam recursos nem vagas.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestIdempotenciaECancelamento(t *testing.T) {
	a := preparar(t)
	c := publicar(t, a, a.motorista, "abc", []string{"A", "B", "C"}, "2099-10-01T08:00:00-03:00", 2)
	repetida := publicar(t, a, a.motorista, "abc", []string{"A", "B", "C"}, "2099-10-01T08:00:00-03:00", 2)
	if c.ID != repetida.ID {
		t.Fatal("publicação duplicada")
	}
	pedido := protocolo.ReservaItinerario{Chave: "mesma", TrechosIDs: ids(c)}
	r, err := a.g.Confirmar(a.passageiro, pedido)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := a.g.Confirmar(a.passageiro, pedido)
	if err != nil || r2.ID != r.ID {
		t.Fatal("reserva duplicada")
	}
	pedido.TrechosIDs = pedido.TrechosIDs[:1]
	if _, err := a.g.Confirmar(a.passageiro, pedido); err == nil {
		t.Fatal("chave reutilizada com outro caminho")
	}
	if _, err := a.g.CancelarReserva(a.outroPassageiro, r.ID); err == nil {
		t.Fatal("terceiro cancelou reserva")
	}
	for i := 0; i < 2; i++ {
		cancelada, err := a.g.CancelarReserva(a.passageiro, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		// A repeticao deve devolver a mesma reserva com todos os dados do cancelamento.
		if cancelada.ID != r.ID || cancelada.Status != protocolo.Cancelada || cancelada.Motivo == "" || cancelada.CanceladaEm == "" {
			t.Fatalf("cancelamento incompleto ou inconsistente: %+v", cancelada)
		}
	}
	caronas, _ := a.g.ConsultarCaronas(a.motorista)
	for _, trecho := range caronas[0].Trechos {
		if trecho.Trecho.Assentos != 2 || len(trecho.Passageiros) != 0 {
			t.Fatal("cancelamento duplicou ou não devolveu vagas")
		}
	}
	reservas, _ := a.g.ConsultarReservas(a.passageiro)
	if len(reservas) != 1 || reservas[0].Status != protocolo.Cancelada {
		t.Fatal("histórico ausente")
	}
	r2, err = a.g.Confirmar(a.passageiro, protocolo.ReservaItinerario{Chave: "mesma", TrechosIDs: ids(c)})
	if err != nil || r2.Status != protocolo.Cancelada {
		t.Fatal("repetição reativou reserva cancelada")
	}
}

// TestRespostaPerdidaPodeSerRecuperada: Fecha a conexao apos enviar a confirmacao, repete a mesma
// chave e verifica que existe somente uma reserva.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestRespostaPerdidaPodeSerRecuperada(t *testing.T) {
	a := preparar(t)
	c := publicar(t, a, a.motorista, "ab", []string{"A", "B"}, "2099-10-01T08:00:00-03:00", 1)
	pedido := protocolo.ReservaItinerario{Chave: "resposta-perdida", TrechosIDs: ids(c)}
	dados, _ := json.Marshal(pedido)
	mensagem, _ := json.Marshal(protocolo.Requisicao{Acao: protocolo.AcaoConfirmar, SessaoID: a.passageiro, Dados: dados})
	servidorConn, clienteConn := net.Pipe()
	fim := make(chan struct{})
	go func() {
		defer close(fim)
		servidor.AtenderConexao(servidorConn, a.g, time.Second)
	}()
	clienteConn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := clienteConn.Write(append(mensagem, '\n')); err != nil {
		t.Fatal(err)
	}
	clienteConn.Close()
	select {
	case <-fim:
	case <-time.After(2 * time.Second):
		t.Fatal("servidor preso no cliente desconectado")
	}
	r, err := a.g.Confirmar(a.passageiro, pedido)
	if err != nil {
		t.Fatal(err)
	}
	reservas, _ := a.g.ConsultarReservas(a.passageiro)
	if len(reservas) != 1 || reservas[0].ID != r.ID {
		t.Fatal("resposta perdida duplicou reserva")
	}
}

// TestCancelamentoConcorrente: Alterna cancelamentos de reserva e carona em goroutines e confere
// que cada vaga foi devolvida uma unica vez.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestCancelamentoConcorrente(t *testing.T) {
	a := preparar(t)
	c := publicar(t, a, a.motorista, "abc", []string{"A", "B", "C"}, "2099-10-01T08:00:00-03:00", 1)
	r, err := a.g.Confirmar(a.passageiro, protocolo.ReservaItinerario{Chave: "r", TrechosIDs: ids(c)})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if i%2 == 0 {
				a.g.CancelarReserva(a.passageiro, r.ID)
			} else {
				a.g.CancelarCarona(a.motorista, c.ID)
			}
		}()
	}
	wg.Wait()
	caronas, _ := a.g.ConsultarCaronas(a.motorista)
	for _, trecho := range caronas[0].Trechos {
		if trecho.Trecho.Assentos != 1 || len(trecho.Passageiros) != 0 {
			t.Fatal("liberação duplicada de assento")
		}
	}
}

// TestCargaTCP: Inicia um servidor TCP local e executa a carga com 50 clientes e tres vagas,
// registrando as metricas.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestCargaTCP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancelar := context.WithCancel(context.Background())
	fim := make(chan error, 1)
	go func() {
		fim <- servidor.Servir(ctx, listener, servidor.NovoGrafo(), 5*time.Second)
	}()
	defer func() {
		cancelar()
		if err := <-fim; err != nil {
			t.Error(err)
		}
	}()
	metricas, err := carga.Executar(listener.Addr().String(), 50, 3)
	raw, _ := json.Marshal(metricas)
	t.Log(string(raw))
	if err != nil {
		t.Fatal(err)
	}
}
