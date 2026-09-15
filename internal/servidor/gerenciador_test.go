/* ================================================================================================
 * internal/servidor/gerenciador_test.go - VaiJunto: sistema de caronas compartilhadas
 * Autor: Arthur Souza
 *
 * Teste de expiracao da sessao: cria um registro com validade no passado e verifica a recusa da
 * consulta.
 *
 * DIVISAO DE RESPONSABILIDADES:
 * Este arquivo prepara cenarios e verifica resultados. As regras exercitadas permanecem nos
 * pacotes da aplicacao.
 * ================================================================================================ */

package servidor

import (
	"testing"
	"time"
	"vaijunto/internal/protocolo"
)

/* TestSessaoExpirada
 *
 * Recebe: t: *testing.T fornecido pelo Go para registrar falhas e mensagens deste teste.
 *
 * O que faz: Insere uma sessao com validade no passado e verifica que a consulta de reservas e
 * recusada.
 *
 * Retorna: Nao retorna valor. Usa t.Fatal, t.Error ou suas variantes para indicar falha nas
 * verificacoes.
 */
func TestSessaoExpirada(t *testing.T) {
	g := NovoGrafo()
	g.usuarios["p@teste.com"] = conta{usuario: protocolo.Usuario{Email: "p@teste.com", Perfil: protocolo.Passageiro}}
	g.sessoes["expirada"] = sessao{email: "p@teste.com", expira: time.Now().Add(-time.Second)}
	if _, err := g.ConsultarReservas("expirada"); err == nil {
		t.Fatal("sessão expirada aceita")
	}
}
