// Arquivo: cmd/cliente_passageiro/main.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Abre o cliente de terminal com o perfil de passageiro.
// Utiliza os menus do pacote cliente para acessar a empresa escolhida pelo usuário.
// Apresenta falhas de inicialização e encerra o programa com código de erro.

package main

import (
	"fmt"
	"os"

	"vaijunto/internal/cliente"
	"vaijunto/internal/protocolo"
)

// main inicia o cliente com o perfil PASSAGEIRO e informa erros no terminal.
// Recebe: nenhum parametro.
// Retorna: nenhum valor; executa os efeitos descritos acima.
func main() {
	if err := cliente.Executar(protocolo.Passageiro); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
