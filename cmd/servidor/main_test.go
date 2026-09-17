// cmd/servidor/main_test.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Testes do transporte: JSON fragmentado, varias linhas, timeout e limite de mensagem.

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

// TestSocketNDJSON: Envia um cadastro fragmentado e outra linha invalida na mesma conexao. Confere
// uma resposta por mensagem.
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
	go func() { fim <- servidor.Servir(ctx, listener, servidor.NovoGrafo(), time.Second) }()
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

// TestTimeoutETamanho: Simula conexao ociosa e mensagem no limite do buffer,
// verificando o encerramento do atendimento.
func TestTimeoutETamanho(t *testing.T) {
	// Executa o mesmo teste para os dois casos; linha incompleta e validada em internal/servidor.
	for _, caso := range []string{"ocioso", "excesso"} {
		//cria um subteste com o nome do caso atual
		t.Run(caso, func(t *testing.T) {
			// cria duas portas de uma conexao
			servidorConn, clienteConn := net.Pipe()
			//cria um canal sem dados para sinal de termino
			fim := make(chan struct{})
			//inicia atendmento do servidor em paralelo
			go func() {
				defer close(fim) // fecha o canal apos goroutine terminar
				//executa uma conexao com o servidor
				servidor.AtenderConexao(servidorConn, servidor.NovoGrafo(), 100*time.Millisecond)
			}()
			//garante que o lado do cliente sera fechado apos subteste
			defer clienteConn.Close()
			//define prazo de max 2 sec para cliente teste
			clienteConn.SetDeadline(time.Now().Add(2 * time.Second))
			switch caso {
			case "excesso":
				// cria uma sequencia de x com o tamanho maximo permitido e envia ao servidor
				if _, err := clienteConn.Write([]byte(strings.Repeat("x", protocolo.LimiteMensagem))); err != nil {
					t.Fatal(err)
				}
				//le reposta enviada pelo servidor ate "\n"
				linha, err := bufio.NewReader(clienteConn).ReadString('\n')
				if err != nil || !strings.Contains(linha, "ERRO") {
					t.Fatalf("limite: %q %v", linha, err)
				}
			}
			select {
			// se fim foi fechado / servidor terminou atendmento corretamente
			case <-fim:
				//se passar 2 sec e servidor nao encerrou
			case <-time.After(2 * time.Second):
				t.Fatal("conexão não liberada")
			}
		})
	}
}
