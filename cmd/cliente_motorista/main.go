package main

import (
	"fmt"
	"os"
	"vaijunto/internal/cliente"
	"vaijunto/internal/protocolo"
)

func main() {
	if err := cliente.Executar(protocolo.Motorista); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
