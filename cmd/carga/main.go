/* ================================================================================================
 * cmd/carga/main.go - VaiJunto: sistema de caronas compartilhadas
 * Autor: Arthur Souza
 *
 * Entrada do teste de carga. Os parametros e as metricas passam pelo terminal.
 *
 * DIVISAO DE RESPONSABILIDADES:
 * Este arquivo inicializa o executavel. O processamento das operacoes fica nos pacotes internal.
 * ================================================================================================ */

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"vaijunto/internal/carga"
	"vaijunto/internal/configuracao"
)

/* main
 *
 * Recebe: Nao recebe parametros Go; a configuracao e lida das flags e variaveis de ambiente do
 * processo.
 *
 * O que faz: le endereco, clientes e vagas. Imprime as metricas em JSON e encerra com erro se o
 * teste falhar.
 *
 * Retorna: Nao retorna valor. Erros fatais sao apresentados no terminal e encerram o programa.
 */
func main() {
	padrao := os.Getenv("VAIJUNTO_SERVIDOR")
	if padrao == "" {
		padrao = configuracao.ServidorPadrao
	}
	servidor := flag.String("servidor", padrao, "IP:porta do servidor TCP")
	clientes := flag.Int("clientes", 100, "Clientes concorrentes, de 1 a 200")
	vagas := flag.Int("vagas", 5, "Vagas em cada um dos dois trechos")
	flag.Parse()
	metricas, err := carga.Executar(*servidor, *clientes, *vagas)
	json.NewEncoder(os.Stdout).Encode(metricas)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
