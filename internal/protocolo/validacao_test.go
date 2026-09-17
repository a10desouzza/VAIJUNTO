// internal/protocolo/validacao_test.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Casos validos e invalidos para os campos compartilhados.

package protocolo

import (
	"strings"
	"testing"
)

// TestValidarEmail: Submete enderecos validos e invalidos ao validador e compara a aceitacao com o
// esperado.
func TestValidarEmail(t *testing.T) {
	for _, email := range []string{"ana@exemplo.com", "ana.silva+carona@alunos.uefs.br", "a@menu.test"} {
		if err := ValidarEmail(email); err != nil {
			t.Errorf("%q: %v", email, err)
		}
	}
	for _, email := range []string{"ana", "ana@", "@exemplo.com", "ana@exemplo", "ana@@exemplo.com", "ana@-exemplo.com", "ana@exemplo..com", "Ana <ana@exemplo.com>", "ana @exemplo.com", "ana@exemplo_com.br", "ana\n@exemplo.com"} {
		if err := ValidarEmail(email); err == nil {
			t.Errorf("aceitou %q", email)
		}
	}
}

// TestValidarNomeSenhaECidade: Verifica os formatos aceitos e recusados para nome, senha e cidade.
func TestValidarNomeSenhaECidade(t *testing.T) {
	for _, nome := range []string{"Ana", "João D'Ávila", "Maria-Clara"} {
		if err := ValidarNome(nome); err != nil {
			t.Error(err)
		}
	}
	for _, nome := range []string{"A", "1234", "Ana123", "--", "Ana\x1b[31m"} {
		if ValidarNome(nome) == nil {
			t.Errorf("nome aceito: %q", nome)
		}
	}
	for _, senha := range []string{"curta", "        ", "senha\x1b123"} {
		if ValidarSenha(senha) == nil {
			t.Errorf("senha inválida aceita")
		}
	}
	for _, cidade := range []string{"Feira de Santana", "São Paulo", "Olho d’Água", "Cidade 2000"} {
		if err := ValidarCidade(cidade); err != nil {
			t.Error(err)
		}
	}
	for _, cidade := range []string{"", "123", "<script>", "Cidade\x1b[2J"} {
		if ValidarCidade(cidade) == nil {
			t.Errorf("cidade aceita: %q", cidade)
		}
	}
}

func TestValidacoesNosLimitesDeTamanhoEUTF8(t *testing.T) {
	if err := ValidarSenha("senha válida 123"); err != nil {
		t.Fatalf("senha válida recusada: %v", err)
	}
	for nome, teste := range map[string]func() error{
		"email acima de 254 bytes":  func() error { return ValidarEmail(strings.Repeat("a", 243) + "@exemplo.com") },
		"nome acima de 100 bytes":   func() error { return ValidarNome(strings.Repeat("a", 101)) },
		"senha acima de 128 bytes":  func() error { return ValidarSenha(strings.Repeat("a", 129)) },
		"cidade acima de 100 bytes": func() error { return ValidarCidade(strings.Repeat("a", 101)) },
		"email com UTF-8 inválido":  func() error { return ValidarEmail("a@exemplo.com" + string([]byte{0xff})) },
		"nome com UTF-8 inválido":   func() error { return ValidarNome("Ana" + string([]byte{0xff})) },
		"senha com UTF-8 inválido":  func() error { return ValidarSenha("senha123" + string([]byte{0xff})) },
		"cidade com UTF-8 inválido": func() error { return ValidarCidade("Cidade" + string([]byte{0xff})) },
	} {
		t.Run(nome, func(t *testing.T) {
			if err := teste(); err == nil {
				t.Fatal("valor inválido foi aceito")
			}
		})
	}
}
