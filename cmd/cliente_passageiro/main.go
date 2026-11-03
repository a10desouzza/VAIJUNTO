// Autor: Arthur de Andrade Souza.
// Matéria: TEC502 - TP01 - Concorrência e Conectividade - 2026.2.
// Declaração de não plágio: Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
// Funcionalidade: Inicia o cliente TCP e os menus disponíveis ao passageiro.
package main

import (
	"fmt"
	"os"
	"vaijunto/internal/cliente"
	"vaijunto/internal/protocolo"
)

// main inicia o cliente com o perfil PASSAGEIRO e informa erros no terminal.
// Recebe: nenhum argumento; lê opções, ambiente e/ou terminal.
func main() {
	if err := cliente.Executar(protocolo.Passageiro); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
