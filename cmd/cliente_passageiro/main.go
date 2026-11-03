// TEC502 - Problema 2. Autor: Arthur de Andrade Souza.
// Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
package main

import (
	"fmt"
	"os"
	"vaijunto/internal/cliente"
	"vaijunto/internal/protocolo"
)

// main inicia o cliente com o perfil PASSAGEIRO e informa erros no terminal.
func main() {
	if err := cliente.Executar(protocolo.Passageiro); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
