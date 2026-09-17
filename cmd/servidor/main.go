// cmd/servidor/main.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Inicializacao do servidor: configura endereco e timeout, abre o socket TCP e trata o
// encerramento.

package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"time"
	"vaijunto/internal/configuracao"
	"vaijunto/internal/servidor"
)

// main: le as flags e inicia um unico estado central. Ctrl+C cancela o contexto do atendimento.
func main() {
	padrao := os.Getenv("VAIJUNTO_ENDERECO")
	if padrao == "" {
		padrao = configuracao.EscutaPadrao
	}
	endereco := flag.String("endereco", padrao, "Endereço TCP de escuta")
	timeout := flag.Duration("timeout", 30*time.Second, "Prazo de leitura e escrita por mensagem")
	flag.Parse()
	if *timeout <= 0 {
		log.Fatal("timeout deve ser positivo")
	}
	listener, err := net.Listen("tcp", *endereco)
	if err != nil {
		log.Fatal(err)
	}
	//contexto utilizado para informar o programa que deve parar
	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt)
	//garante que, quando main terminar, libere os recursos ligados ao sinal
	defer parar()
	log.Printf("VaiJunto escutando em %s", listener.Addr())
	if err := servidor.Servir(ctx, listener, servidor.NovoGrafo(), *timeout); err != nil {
		log.Fatal(err)
	}
}
