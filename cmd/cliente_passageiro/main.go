// cmd/cliente_passageiro/main.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Entrada do cliente passageiro. A interface e a comunicacao ficam no pacote cliente.

package main

import (
	"fmt"
	"os"
	"vaijunto/internal/cliente"
	"vaijunto/internal/protocolo"
)

// main: inicia o cliente com o perfil PASSAGEIRO e informa erros no terminal.
func main() {
	if err := cliente.Executar(protocolo.Passageiro); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
