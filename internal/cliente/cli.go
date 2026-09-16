/* ================================================================================================
 * internal/cliente/cli.go - VaiJunto: sistema de caronas compartilhadas
 * Autor: Arthur Souza
 *
 * Inicializacao do menu interativo com endereco configuravel por flag ou variavel de ambiente.
 *
 * DIVISAO DE RESPONSABILIDADES:
 * O cliente coleta entradas e exibe respostas; o servidor decide permissoes, disponibilidade e
 * alteracoes nas reservas.
 * ================================================================================================ */

package cliente

import (
	"flag"
	"os"
	"vaijunto/internal/configuracao"
)

/* Executar
 *
 * Recebe: perfil: MOTORISTA ou PASSAGEIRO. Tambem le flags e variaveis de ambiente do processo.
 *
 * O que faz: recebe o perfil, interpreta o endereco do servidor e abre o menu interativo.
 *
 * Retorna: nil ao concluir; error de entrada, rede, resposta ou operacao recusada.
 */
func Executar(perfil string) error {
	padrao := os.Getenv("VAIJUNTO_SERVIDOR")
	if padrao == "" {
		padrao = configuracao.ServidorPadrao
	}
	endereco := flag.String("servidor", padrao, "IP:porta do servidor TCP")
	flag.Parse()
	return ExecutarMenu(*endereco, perfil, os.Stdin, os.Stdout)
}
