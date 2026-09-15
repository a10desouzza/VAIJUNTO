/* ================================================================================================
 * cmd/cliente_motorista/main.go - VaiJunto: sistema de caronas compartilhadas
 * Autor: Arthur Souza
 *
 * Entrada do cliente motorista. A interface e a comunicacao ficam no pacote cliente.
 *
 * DIVISAO DE RESPONSABILIDADES:
 * Este arquivo inicializa o executavel. O processamento das operacoes fica nos pacotes internal.
 * ================================================================================================ */

package main

import (
	"fmt"
	"os"
	"vaijunto/internal/cliente"
	"vaijunto/internal/protocolo"
)

/* main
 *
 * Recebe: Nao recebe parametros Go; a configuracao e lida das flags e variaveis de ambiente do
 * processo.
 *
 * O que faz: inicia o cliente com o perfil MOTORISTA e informa erros no terminal.
 *
 * Retorna: Nao retorna valor. Erros fatais sao apresentados no terminal e encerram o programa.
 */
func main() {
	if err := cliente.Executar(protocolo.Motorista); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
