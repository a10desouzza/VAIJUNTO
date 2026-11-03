// TEC502 - Problema 2. Autor: Arthur de Andrade Souza.
// Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
package servidor

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"time"
	"vaijunto/internal/protocolo"
)

// AtenderConexao Le ate a quebra de linha e processa somente mensagens completas. Renova os
// prazos de rede, envia cada resposta e aceita a proxima linha. Uma linha incompleta nao executa a
// operacao; falhar ao enviar a resposta nao desfaz uma operacao que ja foi concluida.
func AtenderConexao(conn net.Conn, g *GrafoItinerarios, timeout time.Duration) {
	atenderConexao(conn, timeout, func(mensagem []byte) []byte {
		g.limparSessoesExpiradas()
		return ProcessarMensagem(mensagem, g)
	})
}

// atenderConexao le mensagens NDJSON completas, chama o processador e envia respostas ate ocorrer falha ou encerramento; fecha a conexao ao sair.
func atenderConexao(conn net.Conn, timeout time.Duration, processar func([]byte) []byte) {
	defer conn.Close()
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	// Requisicoes pequenas nao precisam reservar 1 MiB por conexao.
	leitor := bufio.NewReaderSize(conn, 4096)
	for {
		if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			return
		}
		/* Espera o delimitador. EOF no meio da linha nao executa uma requisicao incompleta. */
		linha, err := lerMensagem(leitor)
		if err != nil {
			if errors.Is(err, bufio.ErrBufferFull) {
				conn.SetWriteDeadline(time.Now().Add(timeout))
				escrever(conn, codificarResposta(protocolo.Resposta{Status: protocolo.Erro, Mensagem: "Mensagem excede o limite de 1 MiB."}))
			}
			return
		}
		resposta := processar(bytes.TrimSuffix(linha, []byte{'\n'}))
		if err := conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
			return
		}
		if err := escrever(conn, resposta); err != nil {
			return
		}
	}
}

// lerMensagem acumula somente quando a linha ultrapassa o buffer inicial.
// Mantem o limite de 1 MiB e nunca executa uma linha incompleta.
func lerMensagem(leitor *bufio.Reader) ([]byte, error) {
	var linha []byte
	for {
		parte, err := leitor.ReadSlice('\n')
		tamanho := len(linha) + len(parte)
		if tamanho > protocolo.LimiteMensagem || tamanho == protocolo.LimiteMensagem && err == bufio.ErrBufferFull {
			return nil, bufio.ErrBufferFull
		}
		if len(linha) == 0 && err == nil {
			return parte, nil
		}
		linha = append(linha, parte...)
		if err != bufio.ErrBufferFull {
			return linha, err
		}
	}
}

// escrever repete Write quando apenas parte do buffer foi enviada, ate transmitir todos os bytes
// ou falhar.
func escrever(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}

// Servir separa o transporte da execucao: leitores aguardam os sockets e um
// trabalhador processa as mensagens. Uma conexao ociosa nao ocupa esse trabalhador.
func Servir(ctx context.Context, listener net.Listener, g *GrafoItinerarios, timeout time.Duration) error {
	ctx, cancelar := context.WithCancel(ctx)
	defer cancelar()
	fila := novaFila()
	fimTrabalhador := make(chan struct {
	})
	go func() {
		defer close(fimTrabalhador)
		fila.atender(ctx, g)
	}()

	var conexoesMu sync.Mutex
	conexoes := make(map[net.Conn]struct{})
	var atendimentos sync.WaitGroup
	vagas := make(chan struct{}, 256)
	fimManutencao := make(chan struct{})
	go func() {
		defer close(fimManutencao)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				g.limparSessoesExpiradas()
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	defer func() {
		cancelar()
		_ = listener.Close()
		conexoesMu.Lock()
		for conn := range conexoes {
			_ = conn.Close()
		}
		conexoesMu.Unlock()
		atendimentos.Wait()
		<-fimTrabalhador
		<-fimManutencao
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case vagas <- struct{}{}:
		default:
			_ = conn.Close()
			continue
		}
		conexoesMu.Lock()
		conexoes[conn] = struct{}{}
		conexoesMu.Unlock()
		atendimentos.Add(1)
		go func() {
			defer atendimentos.Done()
			defer func() {
				<-vagas
				conexoesMu.Lock()
				delete(conexoes, conn)
				conexoesMu.Unlock()
			}()
			atenderConexao(conn, timeout, func(mensagem []byte) []byte {
				return fila.executar(ctx, mensagem)
			})
		}()
	}
}

type requisicaoNaFila struct {
	mensagem []byte
	resposta chan []byte
}

// As duas filas compartilham um trabalhador. Alternar a preferencia impede que
// consultas continuas deixem as escritas esperando, ou vice-versa.
type filaRequisicoes struct {
	leituras chan requisicaoNaFila
	escritas chan requisicaoNaFila
}

// novaFila cria duas filas com capacidade de 128 mensagens cada.
func novaFila() *filaRequisicoes {
	return &filaRequisicoes{
		leituras: make(chan requisicaoNaFila, 128),
		escritas: make(chan requisicaoNaFila, 128),
	}
}

// executar enfileira uma mensagem completa e espera apenas a resposta dela.
func (f *filaRequisicoes) executar(ctx context.Context, mensagem []byte) []byte {
	fila := f.escritas
	if mensagemDeConsulta(mensagem) {
		fila = f.leituras
	}
	pedido := requisicaoNaFila{mensagem: mensagem, resposta: make(chan []byte, 1)}
	select {
	case <-ctx.Done():
		return respostaOcupado("Servidor encerrando.")
	case fila <- pedido:
	default:
		return respostaOcupado("Fila cheia; tente novamente com a mesma chave.")
	}
	select {
	case resposta := <-pedido.resposta:
		return resposta
	case <-ctx.Done():
		return respostaOcupado("Servidor encerrando; consulte o resultado ao reconectar.")
	}
}

// atender e o unico trabalhador das mensagens TCP, alternando leituras e escritas.
func (f *filaRequisicoes) atender(ctx context.Context, g *GrafoItinerarios) {
	preferida, outra := f.leituras, f.escritas
	for {
		if ctx.Err() != nil {
			return
		}
		var pedido requisicaoNaFila
		select {
		case pedido = <-preferida:
		default:
			select {
			case <-ctx.Done():
				return
			case pedido = <-preferida:
			case pedido = <-outra:
			}
		}
		g.limparSessoesExpiradas()
		pedido.resposta <- ProcessarMensagem(pedido.mensagem, g)
		preferida, outra = outra, preferida
	}
}

// respostaOcupado codifica uma resposta de erro para sobrecarga ou encerramento do servidor.
func respostaOcupado(mensagem string) []byte {
	return codificarResposta(protocolo.Resposta{Status: protocolo.Erro, Mensagem: mensagem})
}
