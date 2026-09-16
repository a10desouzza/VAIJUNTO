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
	"encoding/json" // converter dados go <-> JSON
	"flag"          // le opcoes da linha de comando (-servidor, -clientes,...)
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
	padrao := os.Getenv("VAIJUNTO_SERVIDOR") // procura variavel de ambiente
	// se ninguem configurou usa valor padrao
	if padrao == "" {
		padrao = configuracao.ServidorPadrao
	}
	//define 3 flags para que possa rodar o programa com variaveis de ambiente da linha de comando
	servidor := flag.String("servidor", padrao, "IP:porta do servidor TCP")
	clientes := flag.Int("clientes", 100, "Clientes concorrentes, de 1 a 200")
	vagas := flag.Int("vagas", 5, "Vagas em cada um dos dois trechos")
	//processa tudo que o usuario digitou na linha de comando
	flag.Parse()
	//roda testes de carga no servidor
	metricas, err := carga.Executar(*servidor, *clientes, *vagas)
	//imprime metricas em JSON
	json.NewEncoder(os.Stdout).Encode(metricas)
	//verifica se houve erro
	if err != nil {
		//imprime o erro na saida de erro, pra nao misturar com o JSON
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
