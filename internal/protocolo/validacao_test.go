/* ================================================================================================
 * internal/protocolo/validacao_test.go - VaiJunto: sistema de caronas compartilhadas
 * Autor: Arthur Souza
 *
 * Casos validos e invalidos para os campos compartilhados.
 *
 * DIVISAO DE RESPONSABILIDADES:
 * Este arquivo prepara cenarios e verifica resultados. As regras exercitadas permanecem nos
 * pacotes da aplicacao.
 * ================================================================================================ */

package protocolo

import "testing"

/* TestValidarEmail
 *
 * Recebe: t: *testing.T fornecido pelo Go para registrar falhas e mensagens deste teste.
 *
 * O que faz: Submete enderecos validos e invalidos ao validador e compara a aceitacao com o
 * esperado.
 *
 * Retorna: Nao retorna valor. Usa t.Fatal, t.Error ou suas variantes para indicar falha nas
 * verificacoes.
 */
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

/* TestValidarNomeSenhaECidade
 *
 * Recebe: t: *testing.T fornecido pelo Go para registrar falhas e mensagens deste teste.
 *
 * O que faz: Verifica os formatos aceitos e recusados para nome, senha e cidade.
 *
 * Retorna: Nao retorna valor. Usa t.Fatal, t.Error ou suas variantes para indicar falha nas
 * verificacoes.
 */
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
