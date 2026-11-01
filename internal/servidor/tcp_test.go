// Arquivo: internal/servidor/tcp_test.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Testa o transporte TCP com mensagens completas, fragmentadas e acima do limite.
// Simula conexões ociosas, escritas parciais, falhas de escuta e encerramento.
// Verifica que um cliente parado não bloqueia os demais pedidos da fila.

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

// TestConexaoOciosaNaoBloqueiaFilaDeRequisicoes verifica o cenario: Conexao Ociosa Nao Bloqueia Fila De Requisicoes.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestConexaoOciosaNaoBloqueiaFilaDeRequisicoes(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancelar := context.WithCancel(context.Background())
	fim := make(chan error, 1)
	go func() {
		fim <- Servir(ctx, listener, NovoGrafo(), 5*time.Second)
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
	if _, err := io.WriteString(ativa, "{\"acao\":\"CADASTRAR\",\"dados\":{\"nome\":\"Pessoa Teste\",\"email\":\"fila@teste.com\",\"senha\":\"senha1234\",\"perfil\":\"PASSAGEIRO\"}}\n"); err != nil {
		t.Fatal(err)
	}
	linha, err := bufio.NewReader(ativa).ReadBytes('\n')
	if err != nil || !bytes.Contains(linha, []byte(`"status":"SUCESSO"`)) {
		t.Fatalf("conexao ociosa bloqueou outra requisicao: %s %v", linha, err)
	}
}

// TestMensagemMaiorQueBufferInicialRespeitaLimiteDoProtocolo verifica o cenario: Mensagem Maior Que Buffer Inicial Respeita Limite Do Protocolo.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestMensagemMaiorQueBufferInicialRespeitaLimiteDoProtocolo(t *testing.T) {
	conteudo := strings.Repeat(" ", 8192) + `{"acao":"CADASTRAR","dados":{"nome":"Pessoa Teste","email":"buffer@teste.com","senha":"senha1234","perfil":"PASSAGEIRO"}}` + "\n"
	linha, err := lerMensagem(bufio.NewReaderSize(strings.NewReader(conteudo), 4096))
	if err != nil || string(linha) != conteudo {
		t.Fatalf("mensagem dentro do limite recusada: %v", err)
	}
	if r := ProcessarMensagem(bytes.TrimSuffix(linha, []byte{'\n'}), NovoGrafo()); !bytes.Contains(r, []byte(`"status":"SUCESSO"`)) {
		t.Fatalf("mensagem válida não processada: %s", r)
	}
}

// TestConexaoOciosaEncerraNoPrazo verifica o cenario: Conexao Ociosa Encerra No Prazo.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestConexaoOciosaEncerraNoPrazo(t *testing.T) {
	servidorConn, clienteConn := net.Pipe()
	defer clienteConn.Close()
	fim := make(chan struct{})
	go func() {
		defer close(fim)
		AtenderConexao(servidorConn, NovoGrafo(), 100*time.Millisecond)
	}()
	select {
	case <-fim:
	case <-time.After(2 * time.Second):
		t.Fatal("conexao ociosa nao encerrou no prazo")
	}
}

// Write: recebe bytes e simula escrita parcial, ausencia de progresso ou falha configurada.
// Retorna: quantidade de bytes armazenados e o erro simulado, quando houver.
// Recebe: p []byte.
// Retorna: (int, error). Em falha, devolve error; em sucesso, erro nil.
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
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
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
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestAtenderConexaoIgnoraLinhaIncompleta(t *testing.T) {
	g := NovoGrafo()
	servidor, cliente := net.Pipe()
	concluido := make(chan struct{})
	go func() {
		AtenderConexao(servidor, g, time.Second)
		close(concluido)
	}()

	// Envia o cadastro sem a quebra de linha que completa a mensagem do protocolo.
	io.WriteString(cliente, `{"acao":"CADASTRAR","dados":{"nome":"Ana Teste","email":"ana@teste.com","senha":"senha1234","perfil":"PASSAGEIRO"}}`)
	// Fecha a conexao antes de completar a mensagem.
	cliente.Close()
	<-concluido
	if len(g.usuarios) != 0 {
		t.Fatal("requisição sem quebra de linha alterou o estado")
	}
}

// TestAtenderConexaoRejeitaLinhaAcimaDoLimite: recebe o teste e envia uma mensagem grande demais.
// Confere a resposta de erro por limite de tamanho. Sem retorno.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
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

type listenerComErro struct{ err error }

// Accept implementa o comportamento de aceitacao do listener simulado, incluindo erro ou bloqueio conforme o tipo.
// Recebe: nenhum parametro.
// Retorna: (net.Conn, error). Em falha, devolve error; em sucesso, erro nil.
func (l listenerComErro) Accept() (net.Conn, error) {
	return nil, l.err
}

// Close implementa o encerramento do recurso simulado para os testes de transporte.
// Recebe: nenhum parametro.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
func (listenerComErro) Close() error {
	return nil
}

// Addr fornece o endereco do listener simulado.
// Recebe: nenhum parametro.
// Retorna: net.Addr.
func (listenerComErro) Addr() net.Addr {
	return &net.TCPAddr{}
}

// TestServirRetornaErroDeAcceptEEncerraComContexto: recebe o teste e usa listeners simulados.
// Confere propagacao de falha no Accept e encerramento por cancelamento do contexto. Sem retorno.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
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
		listener := &listenerOcioso{fechado: make(chan struct{})}
		resultado := make(chan error, 1)
		go func() {
			resultado <- Servir(ctx, listener, NovoGrafo(), time.Second)
		}()
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

// Listener bloqueante compartilhado pelos testes de encerramento e expiração de sessão.
// Sem rede real, também permite avançar o relógio do synctest sem esperar 15 minutos.
type listenerOcioso struct {
	fechado chan struct{}
	umaVez  sync.Once
}

// Accept implementa o comportamento de aceitacao do listener simulado, incluindo erro ou bloqueio conforme o tipo.
// Recebe: nenhum parametro.
// Retorna: (net.Conn, error). Em falha, devolve error; em sucesso, erro nil.
func (l *listenerOcioso) Accept() (net.Conn, error) {
	<-l.fechado
	return nil, net.ErrClosed
}

// Close implementa o encerramento do recurso simulado para os testes de transporte.
// Recebe: nenhum parametro.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
func (l *listenerOcioso) Close() error {
	l.umaVez.Do(func() {
		close(l.fechado)
	})
	return nil
}

// Addr fornece o endereco do listener simulado.
// Recebe: nenhum parametro.
// Retorna: net.Addr.
func (l *listenerOcioso) Addr() net.Addr {
	return &net.TCPAddr{}
}

// TestServidorRegistraExpiracaoSemNovasRequisicoes: recebe o teste e avanca o relogio simulado.
// Confere que a sessao ociosa expira e gera apenas um registro, sem novos pedidos. Sem retorno.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
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
		go func() {
			fim <- Servir(ctx, listener, g, time.Second)
		}()
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
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestOperacaoTCPNaoRegistraDesconexaoNemEncerraSessao(t *testing.T) {
	c := criarCenario(t)
	var saida bytes.Buffer
	c.g.logger = log.New(&saida, "", 0)
	servidor, cliente := net.Pipe()
	fim := make(chan struct{})
	go func() {
		AtenderConexao(servidor, c.g, time.Second)
		close(fim)
	}()
	cliente.SetDeadline(time.Now().Add(2 * time.Second))
	t.Cleanup(func() {
		cliente.Close()
		<-fim
	})
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

// TestSocketNDJSON: Envia um cadastro fragmentado e outra linha invalida na mesma conexao. Confere
// uma resposta por mensagem.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestSocketNDJSON(t *testing.T) {
	//cria servidor temporario
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	//cria um contexto cancelavel  para encerrar o servidor no final do teste
	ctx, cancelar := context.WithCancel(context.Background())
	//inicia servidor em uma goroutine
	fim := make(chan error, 1)
	go func() {
		fim <- Servir(ctx, listener, NovoGrafo(), time.Second)
	}()
	// garante que o contexto e cancelado e espera a goroutine do servidor encerrar
	defer func() {
		cancelar()
		if err := <-fim; err != nil {
			t.Error(err)
		}
	}()
	//cliente teste se conecta
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	//Garante que a conexao do cliente sera fechada ao final do teste
	defer conn.Close()
	//define prazo se 5 segundos para R/W nessa conexao
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	//laco que percorre duas partes de texto
	for _, parte := range []string{`{"acao":"CADAS`, "TRAR\",\"dados\":{\"nome\":\"Pessoa\",\"email\":\"teste@exemplo.com\",\"senha\":\"senha12345\",\"perfil\":\"PASSAGEIRO\"}}\n{invalido}\n"} {
		if _, err := conn.Write([]byte(parte)); err != nil {
			t.Fatal(err)
		}
	}
	//Cria um leitor com buffer para receber dados do servidor
	reader := bufio.NewReader(conn)

	for _, status := range []string{protocolo.Sucesso, protocolo.Erro} {
		//Ler bytes ate encontar uma quebra de linha
		linha, err := reader.ReadBytes('\n')
		if err != nil {
			t.Fatal(err)
		}
		//Variavel paara guardar resposta deodificada
		var r protocolo.Resposta
		//Coverte a linha JSON recebida
		if err := json.Unmarshal(linha, &r); err != nil {
			t.Fatal(err)
		}
		//Compara status recebido com o status esperado na repetiap
		if r.Status != status || r.Mensagem == "" {
			t.Fatalf("resposta: %s", linha)
		}
	}
}
