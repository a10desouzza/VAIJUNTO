// Arquivo: cmd/cliente_motorista/main.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Abre o cliente de terminal com o perfil de motorista.
// Encaminha a execução para o pacote cliente, onde ficam os menus e as chamadas de rede.
// Apresenta falhas de inicialização e encerra o programa com código de erro.

package main

import (
	"fmt"
	"os"

	"vaijunto/internal/cliente"
	"vaijunto/internal/protocolo"
)

// main inicia o cliente com o perfil MOTORISTA e informa erros no terminal.
// Recebe: nenhum parametro.
// Retorna: nenhum valor; executa os efeitos descritos acima.
func main() {
	if err := cliente.Executar(protocolo.Motorista); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
