// Arquivo: testes/configuracao_test.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Testa a configuração TCP dos clientes e a lista de pares REST do servidor.
// Inclui o erro com duas portas, identidades repetidas e URLs incompatíveis.

package testes

import (
	"vaijunto/internal/configuracao"

	"testing"
)

// TestValidarEnderecoCliente verifica o cenario: Validar Endereco Cliente.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestValidarEnderecoCliente(t *testing.T) {
	for _, endereco := range []string{"172.16.103.7:8080", "localhost:7001", "empresa-a:8080", "[::1]:8080"} {
		if err := configuracao.ValidarEnderecoCliente(endereco); err != nil {
			t.Errorf("%s: %v", endereco, err)
		}
	}
	for _, endereco := range []string{"172.16.103.7:8081:8080", "http://172.16.103.7:8081", "", ":8080", "localhost:0", "localhost:65536", "localhost:abc"} {
		if err := configuracao.ValidarEnderecoCliente(endereco); err == nil {
			t.Errorf("endereco invalido aceito: %s", endereco)
		}
	}
}

// TestEmpresas verifica o cenario: Empresas.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestEmpresas(t *testing.T) {
	for _, texto := range []string{"empresa-a=http://localhost:8001", "b=http://localhost:8002,b=http://localhost:8003", "b=ftp://localhost", "b=http://usuario:senha@localhost", "b=http://localhost/trechos", "b=sem-host"} {
		if _, err := configuracao.Empresas(texto, "empresa-a"); err == nil {
			t.Fatalf("configuracao invalida aceita: %s", texto)
		}
	}
	pares, err := configuracao.Empresas("empresa-b=http://localhost:8002,empresa-c=http://localhost:8003/", "empresa-a")
	if err != nil || len(pares) != 2 || pares[1].URL != "http://localhost:8003" {
		t.Fatalf("pares: %+v; erro: %v", pares, err)
	}
}
