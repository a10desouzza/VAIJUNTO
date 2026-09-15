package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"time"
	"vaijunto/internal/servidor"
)

func main() {
	padrao := os.Getenv("VAIJUNTO_ENDERECO")
	if padrao == "" {
		padrao = ":8080"
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
	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt)
	defer parar()
	log.Printf("VaiJunto escutando em %s", listener.Addr())
	if err := servidor.Servir(ctx, listener, servidor.NovoGrafo(), *timeout); err != nil {
		log.Fatal(err)
	}
}
