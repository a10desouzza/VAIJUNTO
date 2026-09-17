// internal/servidor/tcp_test.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Testes do transporte TCP, encerramento de conexoes e expiracao das sessoes.

package servidor

import (
	"log"
	"testing/synctest"

	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
	"vaijunto/internal/protocolo"
)

type escritorLimitado struct {
	limite int
	dados  []byte
	err    error
	zero   bool
}

// Write: recebe bytes e simula escrita parcial, ausencia de progresso ou falha configurada.
// Retorna: quantidade de bytes armazenados e o erro simulado, quando houver.
func (e *escritorLimitado) Write(p []byte) (int, error) {
	if e.err != nil {
		return 0, e.err
	}
	if e.zero {
		return 0, nil
	}
	n := min(e.limite, len(p))
	e.dados = append(e.dados, p[:n]...)
	return n, nil
}

// TestEscreverCompletaEscritasParciaisEPropagaErros: recebe o teste e simula respostas do escritor.
// Confere envio completo, deteccao de escrita sem progresso e propagacao do erro. Sem retorno.
func TestEscreverCompletaEscritasParciaisEPropagaErros(t *testing.T) {
	t.Run("escritas parciais", func(t *testing.T) {
		e := &escritorLimitado{limite: 2}
		if err := escrever(e, []byte("abcdef")); err != nil {
			t.Fatal(err)
		}
		if string(e.dados) != "abcdef" {
			t.Fatalf("conteúdo escrito = %q", e.dados)
		}
	})

	t.Run("sem progresso", func(t *testing.T) {
		err := escrever(&escritorLimitado{zero: true}, []byte("x"))
		if !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("erro = %v; esperado io.ErrShortWrite", err)
		}
	})

	t.Run("erro do writer", func(t *testing.T) {
		esperado := errors.New("falha de escrita")
		err := escrever(&escritorLimitado{err: esperado}, []byte("x"))
		if !errors.Is(err, esperado) {
			t.Fatalf("erro = %v; esperado %v", err, esperado)
		}
	})
}

// TestAtenderConexaoIgnoraLinhaIncompleta: recebe o teste e fecha o pipe sem enviar a quebra de linha.
// Confere que o cadastro incompleto nao altera o estado do servidor. Sem retorno.
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

// TestAtenderConexaoRejeitaLinhaAcimaDoLimite: recebe o teste e envia uma mensagem grande demais.
// Confere a resposta de erro por limite de tamanho. Sem retorno.
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

type enderecoFalso string

func (e enderecoFalso) Network() string { return "teste" }
func (e enderecoFalso) String() string  { return string(e) }

type listenerComErro struct{ err error }

func (l listenerComErro) Accept() (net.Conn, error) { return nil, l.err }
func (listenerComErro) Close() error                { return nil }
func (listenerComErro) Addr() net.Addr              { return enderecoFalso("teste") }

type listenerBloqueado struct {
	fechado chan struct{}
	umaVez  sync.Once
}

func (l *listenerBloqueado) Accept() (net.Conn, error) {
	<-l.fechado
	return nil, net.ErrClosed
}
func (l *listenerBloqueado) Close() error {
	l.umaVez.Do(func() { close(l.fechado) })
	return nil
}
func (*listenerBloqueado) Addr() net.Addr { return enderecoFalso("teste") }

// TestServirRetornaErroDeAcceptEEncerraComContexto: recebe o teste e usa listeners simulados.
// Confere propagacao de falha no Accept e encerramento por cancelamento do contexto. Sem retorno.
func TestServirRetornaErroDeAcceptEEncerraComContexto(t *testing.T) {
	t.Run("erro de accept", func(t *testing.T) {
		esperado := errors.New("accept falhou")
		err := Servir(context.Background(), listenerComErro{err: esperado}, NovoGrafo(), time.Second)
		if !errors.Is(err, esperado) {
			t.Fatalf("erro = %v; esperado %v", err, esperado)
		}
	})

	t.Run("cancelamento", func(t *testing.T) {
		ctx, cancelar := context.WithCancel(context.Background())
		listener := &listenerBloqueado{fechado: make(chan struct{})}
		resultado := make(chan error, 1)
		go func() { resultado <- Servir(ctx, listener, NovoGrafo(), time.Second) }()
		cancelar()
		select {
		case err := <-resultado:
			if err != nil {
				t.Fatalf("Servir retornou erro ao cancelar: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("Servir não encerrou após cancelar o contexto")
		}
	})
}

// Listener sem rede real permite adiantar 15 minutos com o relogio do synctest.
type listenerOcioso struct {
	fechado chan struct{}
	umaVez  sync.Once
}

func (l *listenerOcioso) Accept() (net.Conn, error) { <-l.fechado; return nil, net.ErrClosed }
func (l *listenerOcioso) Close() error              { l.umaVez.Do(func() { close(l.fechado) }); return nil }
func (l *listenerOcioso) Addr() net.Addr            { return &net.TCPAddr{} }

// TestServidorRegistraExpiracaoSemNovasRequisicoes: recebe o teste e avanca o relogio simulado.
// Confere que a sessao ociosa expira e gera apenas um registro, sem novos pedidos. Sem retorno.
func TestServidorRegistraExpiracaoSemNovasRequisicoes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		g := NovoGrafo()
		var saida bytes.Buffer
		g.logger = log.New(&saida, "", 0)
		g.sessoes["sessao-teste"] = sessao{email: "ocioso@teste.com", expira: time.Now().Add(15 * time.Minute)}
		listener := &listenerOcioso{fechado: make(chan struct{})}
		ctx, cancelar := context.WithCancel(context.Background())
		defer cancelar()
		fim := make(chan error, 1)
		go func() { fim <- Servir(ctx, listener, g, time.Second) }()
		synctest.Wait()
		time.Sleep(15*time.Minute + time.Second)
		synctest.Wait()
		g.mu.RLock()
		restantes := len(g.sessoes)
		g.mu.RUnlock()
		if restantes != 0 || strings.Count(saida.String(), "DESCONECTOU usuário=ocioso@teste.com motivo=sessão expirada (15 minutos)") != 1 {
			t.Fatalf("expiração ociosa: %d %s", restantes, &saida)
		}
		cancelar()
		if err := <-fim; err != nil {
			t.Fatal(err)
		}
	})
}

// TestOperacaoTCPNaoRegistraDesconexaoNemEncerraSessao: recebe o teste e fecha um socket apos consultar.
// Confere que a sessao continua valida e nao houve registro de logout. Sem retorno.
func TestOperacaoTCPNaoRegistraDesconexaoNemEncerraSessao(t *testing.T) {
	c := criarCenario(t)
	var saida bytes.Buffer
	c.g.logger = log.New(&saida, "", 0)
	servidor, cliente := net.Pipe()
	fim := make(chan struct{})
	go func() { AtenderConexao(servidor, c.g, time.Second); close(fim) }()
	cliente.SetDeadline(time.Now().Add(2 * time.Second))
	t.Cleanup(func() { cliente.Close(); <-fim })
	req := protocolo.Requisicao{Acao: protocolo.AcaoReservas, SessaoID: c.p, Dados: json.RawMessage(`{}`)}
	if err := json.NewEncoder(cliente).Encode(req); err != nil {
		t.Fatal(err)
	}
	if _, err := bufio.NewReader(cliente).ReadBytes('\n'); err != nil {
		t.Fatal(err)
	}
	cliente.Close()
	<-fim
	if saida.Len() != 0 {
		t.Fatal(saida.String())
	}
	if _, err := c.g.ConsultarReservas(c.p); err != nil {
		t.Fatal("fechar socket invalidou sessão", err)
	}
}
