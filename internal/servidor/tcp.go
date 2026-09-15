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

func Servir(ctx context.Context, listener net.Listener, g *GrafoItinerarios, timeout time.Duration) error {
	var wg sync.WaitGroup
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
