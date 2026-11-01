// Arquivo: internal/servidor/tcp.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Aceita conexões TCP e delimita as mensagens JSON pela quebra de linha.
// Controla prazos, tamanho das mensagens e envio completo das respostas.
// Leitores de rede encaminham pedidos ao executor para que conexões ociosas não o bloqueiem.

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
// Recebe: conn net.Conn, g *GrafoItinerarios, timeout time.Duration.
// Retorna: nenhum valor; executa os efeitos descritos acima.
func AtenderConexao(conn net.Conn, g *GrafoItinerarios, timeout time.Duration) {
	atenderConexao(conn, timeout, func(mensagem []byte) []byte {
		g.limparSessoesExpiradas()
		return ProcessarMensagem(mensagem, g)
	})
}

// atenderConexao le mensagens NDJSON completas, chama o processador e envia respostas ate ocorrer falha ou encerramento; fecha a conexao ao sair.
// Recebe: conn net.Conn, timeout time.Duration, processar func([]byte) []byte.
// Retorna: nenhum valor; executa os efeitos descritos acima.
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
// Recebe: leitor *bufio.Reader.
// Retorna: a linha recebida e eventual erro de leitura/limite; o chamador nao deve processar linha incompleta.
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
// Recebe: w io.Writer, b []byte.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
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
// Recebe: ctx context.Context, listener net.Listener, g *GrafoItinerarios, timeout time.Duration.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
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
