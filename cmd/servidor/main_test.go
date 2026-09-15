package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"
	"vaijunto/internal/protocolo"
	"vaijunto/internal/servidor"
)

func TestSocketNDJSON(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancelar := context.WithCancel(context.Background())
	fim := make(chan error, 1)
	go func() { fim <- servidor.Servir(ctx, listener, servidor.NovoGrafo(), time.Second) }()
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
		if r.Status != status {
			t.Fatalf("resposta: %s", linha)
		}
	}
}

func TestTimeoutETamanho(t *testing.T) {
	for _, caso := range []string{"ocioso", "excesso", "incompleta"} {
		t.Run(caso, func(t *testing.T) {
			servidorConn, clienteConn := net.Pipe()
			fim := make(chan struct{})
			go func() {
				defer close(fim)
				servidor.AtenderConexao(servidorConn, servidor.NovoGrafo(), 100*time.Millisecond)
			}()
			defer clienteConn.Close()
			clienteConn.SetDeadline(time.Now().Add(2 * time.Second))
			switch caso {
			case "excesso":
				if _, err := clienteConn.Write([]byte(strings.Repeat("x", protocolo.LimiteMensagem))); err != nil {
					t.Fatal(err)
				}
				linha, err := bufio.NewReader(clienteConn).ReadString('\n')
				if err != nil || !strings.Contains(linha, "ERRO") {
					t.Fatalf("limite: %q %v", linha, err)
				}
			case "incompleta":
				clienteConn.Write([]byte(`{"acao":`))
				clienteConn.Close()
			}
			select {
			case <-fim:
			case <-time.After(2 * time.Second):
				t.Fatal("conexão não liberada")
			}
		})
	}
}
