package servidor

import (
	"testing"
	"time"
	"vaijunto/internal/protocolo"
)

func TestSessaoExpirada(t *testing.T) {
	g := NovoGrafo()
	g.usuarios["p@teste.com"] = conta{usuario: protocolo.Usuario{Email: "p@teste.com", Perfil: protocolo.Passageiro}}
	g.sessoes["expirada"] = sessao{email: "p@teste.com", expira: time.Now().Add(-time.Second)}
	if _, err := g.ConsultarReservas("expirada"); err == nil {
		t.Fatal("sessão expirada aceita")
	}
}
