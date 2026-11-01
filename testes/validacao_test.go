// Arquivo: testes/validacao_test.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Testa os valores aceitos e recusados pelas validações de cadastro e cidade.
// Cobre limites de tamanho e textos com representação UTF-8 inválida.

package testes

import (
	"vaijunto/internal/protocolo"

	"strings"
	"testing"
)

// TestValidarEmail: Submete enderecos validos e invalidos ao validador e compara a aceitacao com o
// esperado.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestValidarEmail(t *testing.T) {
	for _, email := range []string{"ana@exemplo.com", "ana.silva+carona@alunos.uefs.br", "a@menu.test"} {
		if err := protocolo.ValidarEmail(email); err != nil {
			t.Errorf("%q: %v", email, err)
		}
	}
	for _, email := range []string{"ana", "ana@", "@exemplo.com", "ana@exemplo", "ana@@exemplo.com", "ana@-exemplo.com", "ana@exemplo..com", "Ana <ana@exemplo.com>", "ana @exemplo.com", "ana@exemplo_com.br", "ana\n@exemplo.com"} {
		if err := protocolo.ValidarEmail(email); err == nil {
			t.Errorf("aceitou %q", email)
		}
	}
}

// TestValidarNomeSenhaECidade: Verifica os formatos aceitos e recusados para nome, senha e cidade.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestValidarNomeSenhaECidade(t *testing.T) {
	for _, nome := range []string{"Ana", "João D'Ávila", "Maria-Clara"} {
		if err := protocolo.ValidarNome(nome); err != nil {
			t.Error(err)
		}
	}
	for _, nome := range []string{"A", "1234", "Ana123", "--", "Ana\x1b[31m"} {
		if protocolo.ValidarNome(nome) == nil {
			t.Errorf("nome aceito: %q", nome)
		}
	}
	for _, senha := range []string{"curta", "        ", "senha\x1b123"} {
		if protocolo.ValidarSenha(senha) == nil {
			t.Errorf("senha inválida aceita")
		}
	}
	for _, cidade := range []string{"Feira de Santana", "São Paulo", "Olho d’Água", "Cidade 2000"} {
		if err := protocolo.ValidarCidade(cidade); err != nil {
			t.Error(err)
		}
	}
	for _, cidade := range []string{"", "123", "<script>", "Cidade\x1b[2J"} {
		if protocolo.ValidarCidade(cidade) == nil {
			t.Errorf("cidade aceita: %q", cidade)
		}
	}
}

// TestValidacoesNosLimitesDeTamanhoEUTF8 verifica o cenario: Validacoes Nos Limites De Tamanho EUTF8.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestValidacoesNosLimitesDeTamanhoEUTF8(t *testing.T) {
	if err := protocolo.ValidarSenha("senha válida 123"); err != nil {
		t.Fatalf("senha válida recusada: %v", err)
	}
	for nome, teste := range map[string]func() error{
		"email acima de 254 bytes": func() error {
			return protocolo.ValidarEmail(strings.Repeat("a", 243) + "@exemplo.com")
		},
		"nome acima de 100 bytes": func() error {
			return protocolo.ValidarNome(strings.Repeat("a", 101))
		},
		"senha acima de 128 bytes": func() error {
			return protocolo.ValidarSenha(strings.Repeat("a", 129))
		},
		"cidade acima de 100 bytes": func() error {
			return protocolo.ValidarCidade(strings.Repeat("a", 101))
		},
		"email com UTF-8 inválido": func() error {
			return protocolo.ValidarEmail("a@exemplo.com" + string([]byte{0xff}))
		},
		"nome com UTF-8 inválido": func() error {
			return protocolo.ValidarNome("Ana" + string([]byte{0xff}))
		},
		"senha com UTF-8 inválido": func() error {
			return protocolo.ValidarSenha("senha123" + string([]byte{0xff}))
		},
		"cidade com UTF-8 inválido": func() error {
			return protocolo.ValidarCidade("Cidade" + string([]byte{0xff}))
		},
	} {
		t.Run(nome, func(t *testing.T) {
			if err := teste(); err == nil {
				t.Fatal("valor inválido foi aceito")
			}
		})
	}
}
