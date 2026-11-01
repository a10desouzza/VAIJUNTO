// Arquivo: cmd/carga/main.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Recebe as opções para simular vários passageiros e disputar vagas por TCP.
// Escolhe a simulação local ou entre empresas e apresenta suas métricas em JSON.
// Permite avaliar o sistema sem preencher os formulários dos clientes manualmente.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"vaijunto/internal/carga"
	"vaijunto/internal/configuracao"
)

// main le endereco, clientes e vagas. Imprime as metricas em JSON e encerra com erro se o teste
// falhar.
// Recebe: nenhum parametro.
// Retorna: nenhum valor; executa os efeitos descritos acima.
func main() {
	padrao := os.Getenv("VAIJUNTO_SERVIDOR")
	if padrao == "" {
		padrao = configuracao.ServidorPadrao
	}
	servidor := flag.String("servidor", padrao, "IP:porta do servidor TCP")
	servidores := flag.String("servidores", "", "Três endereços TCP separados por vírgula para carga do consórcio")
	clientes := flag.Int("clientes", 100, "Clientes concorrentes, de 1 a 200")
	vagas := flag.Int("vagas", 5, "Vagas em cada um dos dois trechos")
	flag.Parse()
	var metricas carga.Metricas
	var err error
	if *servidores != "" {
		enderecos := strings.Split(*servidores, ",")
		for i := range enderecos {
			enderecos[i] = strings.TrimSpace(enderecos[i])
		}
		metricas, err = carga.ExecutarConsorcio(enderecos, *clientes, *vagas)
	} else {
		metricas, err = carga.Executar(*servidor, *clientes, *vagas)
	}
	if encodeErr := json.NewEncoder(os.Stdout).Encode(metricas); encodeErr != nil {
		fmt.Fprintln(os.Stderr, encodeErr)
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
