/* ================================================================================================
 * internal/servidor/tcp.go - VaiJunto: sistema de caronas compartilhadas
 * Autor: Arthur Souza
 *
 * Atendimento TCP por goroutine, com limite de conexoes e prazos de leitura e escrita.
 * A quebra de linha delimita a mensagem; uma leitura TCP pode conter apenas parte dela.
 *
 * DIVISAO DE RESPONSABILIDADES:
 * O servidor mantem o estado em RAM e valida as operacoes. As estruturas compartilhadas sao
 * protegidas por travas.
 * ================================================================================================ */

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

/* AtenderConexao
 *
 * Recebe: conn: socket aceito; g: estado central compartilhado; timeout: prazo por leitura/escrita.
 *
 * O que faz: Le ate a quebra de linha e processa somente mensagens completas. Renova os prazos de
 * rede,
 * envia cada resposta e aceita a proxima linha. Uma linha incompleta nao executa a operacao;
 * falhar ao enviar a resposta nao desfaz uma operacao que ja foi concluida.
 *
 * Retorna: Nao retorna valor. Fecha o socket ao sair, por EOF, timeout ou erro.
 */
func AtenderConexao(conn net.Conn, g *GrafoItinerarios, timeout time.Duration) {
	defer conn.Close()
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	leitor := bufio.NewReaderSize(conn, protocolo.LimiteMensagem)
	for {
		if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			return
		}
		/* Espera o delimitador. EOF no meio da linha nao executa uma requisicao incompleta. */
		linha, err := leitor.ReadSlice('\n')
		if err != nil {
			if errors.Is(err, bufio.ErrBufferFull) {
				conn.SetWriteDeadline(time.Now().Add(timeout))
				escrever(conn, codificarResposta(protocolo.Resposta{Status: protocolo.Erro, Mensagem: "Mensagem excede o limite de 1 MiB."}))
			}
			return
		}
		resposta := ProcessarMensagem(bytes.TrimSuffix(linha, []byte{'\n'}), g)
		if err := conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
			return
		}
		if err := escrever(conn, resposta); err != nil {
			return
		}
	}
}

/* escrever
 *
 * Recebe: w: destino que implementa io.Writer; b: bytes que precisam ser enviados.
 *
 * O que faz: repete Write quando apenas parte do buffer foi enviada, ate transmitir todos os bytes
 * ou falhar.
 *
 * Retorna: nil depois de enviar todos os bytes; error de escrita ou io.ErrShortWrite se nao houver
 * progresso.
 */
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

/* Servir
 *
 * Recebe: ctx: sinal de encerramento; listener: socket de escuta ja aberto; g: estado central;
 * timeout: prazo de I/O.
 *
 * O que faz: Aceita sockets e limita a 256 atendimentos ativos por um canal usado como semaforo.
 * Cada conexao roda em uma goroutine. No encerramento fecha o listener e os sockets registrados,
 * depois espera o WaitGroup para que os atendimentos terminem.
 *
 * Retorna: nil ao encerrar pelo contexto; error se a aceitacao falhar sem cancelamento.
 */
func Servir(ctx context.Context, listener net.Listener, g *GrafoItinerarios, timeout time.Duration) error {
	var wg sync.WaitGroup
	/* Esta trava cuida apenas da lista de sockets. O grafo possui seu proprio RWMutex. */
	var mu sync.Mutex
	conexoes := make(map[net.Conn]struct{})
	encerrar := make(chan struct{})
	defer close(encerrar)
	go func() {
		select {
		case <-ctx.Done():
			listener.Close()
		case <-encerrar:
		}
	}()
	defer func() {
		listener.Close()
		mu.Lock()
		for conn := range conexoes {
			conn.Close()
		}
		mu.Unlock()
		wg.Wait()
	}()
	/* Canal como semaforo: limita atendimentos ativos e devolve a vaga quando a goroutine sai. */
	vagas := make(chan struct{}, 256)
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
			conn.Close()
			continue
		}
		mu.Lock()
		conexoes[conn] = struct{}{}
		mu.Unlock()
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-vagas; mu.Lock(); delete(conexoes, conn); mu.Unlock() }()
			AtenderConexao(conn, g, timeout)
		}()
	}
}
