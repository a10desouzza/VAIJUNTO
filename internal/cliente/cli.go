// internal/cliente/cli.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Inicializacao do menu interativo com endereco configuravel por flag ou variavel de ambiente.

package cliente

import (
	"flag"
	"os"
	"vaijunto/internal/configuracao"
)

// Executar: recebe o perfil, interpreta o endereco do servidor e abre o menu interativo. Retorna:
// nil ao concluir; error de entrada, rede, resposta ou operacao recusada.
func Executar(perfil string) error {
	padrao := os.Getenv("VAIJUNTO_SERVIDOR") // busca endereco do servidor em variavel de ambiente
	// se a variavel nao estiver definida, usa o endereco padrao do arquivo de configuracao
	if padrao == "" {
		padrao = configuracao.ServidorPadrao
	}
	// cria flag de linha de comando para o endereco do servidor, com valor padrao definido acima
	endereco := flag.String("servidor", padrao, "IP:porta do servidor TCP")
	flag.Parse()                                                // le as flags digitadas no terminal
	return ExecutarMenu(*endereco, perfil, os.Stdin, os.Stdout) // abre o menu
}
