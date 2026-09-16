package servidor

import (
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

func TestAtenderConexaoProcessaVariasLinhasNoMesmoPipe(t *testing.T) {
	servidor, cliente := net.Pipe()
	concluido := make(chan struct{})
	go func() {
		AtenderConexao(servidor, NovoGrafo(), time.Second)
		close(concluido)
	}()
	t.Cleanup(func() {
		cliente.Close()
		<-concluido
	})

	if _, err := io.WriteString(cliente, "{\"acao\":\"DESCONHECIDA\",\"dados\":{}}\n{\"acao\":\"DESCONHECIDA\",\"dados\":{}}\n"); err != nil {
		t.Fatal(err)
	}
	leitor := bufio.NewReader(cliente)
	for i := 0; i < 2; i++ {
		linha, err := leitor.ReadBytes('\n')
		if err != nil {
			t.Fatal(err)
		}
		var resposta protocolo.Resposta
		if err := decodificar(linha, &resposta); err != nil {
			t.Fatalf("resposta %d inválida: %v", i, err)
		}
		if resposta.Status != protocolo.Erro || resposta.Mensagem == "" {
			t.Fatalf("resposta %d = %+v", i, resposta)
		}
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
