package cliente

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
	"vaijunto/internal/protocolo"
)

func TestCamposRepetemAteValorValido(t *testing.T) {
	var saida bytes.Buffer
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
