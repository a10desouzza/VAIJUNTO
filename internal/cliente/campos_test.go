// internal/cliente/campos_test.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Testes de campos invalidos e retorno ao menu antes do envio.

package cliente

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"
	"vaijunto/internal/protocolo"
)

func TestPublicarPerguntaParadaSomenteNasCidadesIntermediarias(t *testing.T) {
	//testa 4 situacoes
	for _, n := range []int{2, 3, 8, 21} {
		t.Run(fmt.Sprint(n), func(t *testing.T) { // cria um subteste para cada valor de n
			var entrada strings.Builder // cria uma entrada simulada para o menu
			fmt.Fprintln(&entrada, n)   // escreve n dentro de entrada, seguido de uma quebra de linha
			// cria n cidades
			for i := 0; i < n; i++ {
				fmt.Fprintf(&entrada, "Cidade %c\n", 'A'+i)
			}
			fmt.Fprint(&entrada, "2099-10-01\n08:00\n3\n")
			// cria n-1 trechos
			for i := 0; i < n-1; i++ {
				// par cd trecho, simula resposta
				fmt.Fprint(&entrada, "10\n20\n60\n")
				if i < n-2 { // tem que ter pelo menos 2 trechos para perguntar sobre paradas
					fmt.Fprintln(&entrada, "15")
				}
			}
			// Desiste na confirmação; não precisa de servidor nem de conexão.
			fmt.Fprint(&entrada, "0\nproxima entrada\n")
			var saida bytes.Buffer // cria uma "tela falsa", tudo que o menu mostraria sera escrito em saida
			// cria um menu de teste
			m := &menu{leitor: bufio.NewScanner(strings.NewReader(entrada.String())), saida: &saida}
			//executa a publicação e verifica se houve erro
			if err := m.publicar(); err != nil || m.err != nil {
				t.Fatalf("publicar: %v; entrada: %v", err, m.err)
			}
			if got := strings.Count(saida.String(), "Parada após este trecho em minutos"); got != n-2 {
				t.Fatalf("perguntas de parada: %d; esperado %d", got, n-2)
			}
			if got := strings.Count(saida.String(), "min de parada"); got != n-2 {
				t.Fatalf("resumo de paradas: %d; esperado %d", got, n-2)
			}
			if !m.leitor.Scan() || m.leitor.Text() != "proxima entrada" {
				t.Fatal("consumiu entrada além da confirmação")
			}
		})
	}
}

// TestCamposRepetemAteValorValido: Fornece valores invalidos seguidos dos validos e verifica a
// repeticao dos campos e as mensagens de erro.
func TestCamposRepetemAteValorValido(t *testing.T) {
	var saida bytes.Buffer
	// cria um menu de teste com entradas invalidas e validas
	m := &menu{leitor: bufio.NewScanner(strings.NewReader("invalido\nana@dominio\nANA@EXEMPLO.COM\n123\nSalvador\nFeira de Santana\n31/02/2026\n01/10/2099\nNaN\n1e3\n25,555\n25,50\n")), saida: &saida}
	if email := m.email(); email != "ana@exemplo.com" {
		t.Fatalf("email: %q", email)
	}
	if cidade := m.cidade("Destino", []string{"salvador"}); cidade != "Feira de Santana" {
		t.Fatalf("cidade: %q", cidade)
	}
	if data := m.data(); data != "2099-10-01" {
		t.Fatalf("data: %q", data)
	}
	if preco := m.preco(); preco != 25.50 {
		t.Fatalf("preco: %v", preco)
	}
	if m.err != nil {
		t.Fatal(m.err)
	}
	for _, trecho := range []string{"e-mail válido", "cidade já foi informada", "Data inexistente"} {
		if !strings.Contains(saida.String(), trecho) {
			t.Errorf("mensagem ausente: %s", trecho)
		}
	}
}

// TestVoltarAbandonaCadastroSemRede: Interrompe o cadastro com /voltar e confere que nao houve
// tentativa de envio nem codigos de cor na saida.
func TestVoltarAbandonaCadastroSemRede(t *testing.T) {
	var saida bytes.Buffer
	err := ExecutarMenu("127.0.0.1:1", protocolo.Passageiro, strings.NewReader("2\nAna\n/voltar\n0\n"), &saida)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(saida.String(), "Você voltou ao menu") || strings.Contains(saida.String(), "Não foi possível obter") {
		t.Fatal(saida.String())
	}
	if strings.Contains(saida.String(), "\x1b[") {
		t.Fatal("cores ANSI em saída redirecionada")
	}
}
