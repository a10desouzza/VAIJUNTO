/* ================================================================================================
 * internal/cliente/campos_test.go - VaiJunto: sistema de caronas compartilhadas
 * Autor: Arthur Souza
 *
 * Testes de campos invalidos e retorno ao menu antes do envio.
 *
 * DIVISAO DE RESPONSABILIDADES:
 * Este arquivo prepara cenarios e verifica resultados. As regras exercitadas permanecem nos
 * pacotes da aplicacao.
 * ================================================================================================ */

package cliente

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
	"vaijunto/internal/protocolo"
)

/* TestCamposRepetemAteValorValido
 *
 * Recebe: t: *testing.T fornecido pelo Go para registrar falhas e mensagens deste teste.
 *
 * O que faz: Fornece valores invalidos seguidos dos validos e verifica a repeticao dos campos e as
 * mensagens de erro.
 *
 * Retorna: Nao retorna valor. Usa t.Fatal, t.Error ou suas variantes para indicar falha nas
 * verificacoes.
 */
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

/* TestVoltarAbandonaCadastroSemRede
 *
 * Recebe: t: *testing.T fornecido pelo Go para registrar falhas e mensagens deste teste.
 *
 * O que faz: Interrompe o cadastro com /voltar e confere que nao houve tentativa de envio nem
 * codigos de cor na saida.
 *
 * Retorna: Nao retorna valor. Usa t.Fatal, t.Error ou suas variantes para indicar falha nas
 * verificacoes.
 */
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
