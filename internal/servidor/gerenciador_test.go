// internal/servidor/gerenciador_test.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Teste de validade da sessao, recusa de consultas expiradas e registros sem segredos.

package servidor

import (
	"bytes"
	"log"
	"strings"

	"testing"
	"time"
	"vaijunto/internal/protocolo"
)

// TestSessaoQuinzeMinutosELogsSemSegredos: recebe o teste e avanca o relogio da sessao.
// Confere consultas antes, no instante e depois da expiracao, limpeza e logs. Sem retorno.
func TestSessaoQuinzeMinutosELogsSemSegredos(t *testing.T) {
	c := criarCenario(t)
	var saida bytes.Buffer
	c.g.logger = log.New(&saida, "", 0)
	s, err := c.g.Autenticar(protocolo.Credenciais{Email: "p@teste.com", Senha: "senha1234"})
	if err != nil {
		t.Fatal(err)
	}
	expira, err := time.Parse(time.RFC3339, s.ExpiraEm)
	if err != nil || expira.Sub(c.agora) != 15*time.Minute {
		t.Fatalf("validade: %s %v", s.ExpiraEm, err)
	}
	if err := c.g.Desconectar(c.outro); err != nil {
		t.Fatal(err)
	}
	c.agora = expira.Add(-time.Second)
	if _, err := c.g.ConsultarReservas(s.ID); err != nil {
		t.Fatal(err)
	}
	c.agora = expira
	if _, err := c.g.ConsultarReservas(s.ID); err == nil {
		t.Fatal("sessão aceita no limite de expiração")
	}
	c.agora = expira.Add(time.Second)
	if _, err := c.g.ConsultarReservas(s.ID); err == nil {
		t.Fatal("sessão expirada aceita")
	}
	c.g.limparSessoesExpiradas()
	tamanho := saida.Len()
	c.g.limparSessoesExpiradas()
	if saida.Len() != tamanho {
		t.Fatal("expiração registrada novamente")
	}
	for _, esperado := range []string{"CONECTOU usuário=p@teste.com", "DESCONECTOU usuário=outro@teste.com", "DESCONECTOU usuário=p@teste.com motivo=sessão expirada (15 minutos)"} {
		if !strings.Contains(saida.String(), esperado) {
			t.Fatalf("log ausente %q: %s", esperado, &saida)
		}
	}
	if strings.Contains(saida.String(), s.ID) || strings.Contains(saida.String(), "senha1234") {
		t.Fatal("segredo no log")
	}
	if len(c.g.sessoes) != 0 {
		t.Fatal("sessões expiradas mantidas")
	}
}
