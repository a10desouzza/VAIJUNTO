package servidor

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"vaijunto/internal/protocolo"
)

func TestSenhasComSaltEComparacao(t *testing.T) {
	g := NovoGrafo()
	for _, email := range []string{"ana@teste.com", "bia@teste.com"} {
		if _, err := g.Cadastrar(protocolo.Cadastro{Nome: "Pessoa Teste", Email: email, Senha: "senha1234", Perfil: protocolo.Passageiro}); err != nil {
			t.Fatal(err)
		}
	}
	a, b := g.usuarios["ana@teste.com"].senha, g.usuarios["bia@teste.com"].senha
	if a == b || strings.Contains(a, "senha1234") || !conferirSenha("senha1234", a) || conferirSenha("incorreta", a) {
		t.Fatal("salt ou verificacao incorreta")
	}
	for _, invalido := range []string{"senha1234", "pbkdf2-sha256$1$00$00", "pbkdf2-sha256$600000$zz$00", a[:len(a)-1]} {
		if conferirSenha("senha1234", invalido) {
			t.Fatal("hash malformado aceito")
		}
	}
}

func TestBackupMigraSenhasLegadasSemPerderReservas(t *testing.T) {
	c := criarCenario(t)
	c.reservar(t)
	caminho := filepath.Join(t.TempDir(), "estado.json")
	if err := c.g.AbrirBackup("local", caminho); err != nil {
		t.Fatal(err)
	}
	ler := func() arquivoBackup {
		t.Helper()
		b, err := os.ReadFile(caminho)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(b, []byte("senha1234")) {
			t.Fatal("senha original persistida")
		}
		var a arquivoBackup
		if err := json.Unmarshal(b, &a); err != nil {
			t.Fatal(err)
		}
		return a
	}
	arquivo := ler()
	var estado estadoBackup
	if err := json.Unmarshal(arquivo.Estado, &estado); err != nil {
		t.Fatal(err)
	}
	gravar := func() {
		t.Helper()
		arquivo.Estado, _ = json.Marshal(estado)
		arquivo.Checksum = hashBytes(arquivo.Estado)
		b, _ := json.Marshal(arquivo)
		if err := os.WriteFile(caminho, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Simula o formato legado do projeto, com senha em texto puro.
	conta := estado.Usuarios["p@teste.com"]
	conta.Senha = "senha1234"
	estado.Usuarios["p@teste.com"] = conta
	gravar()
	novo := NovoGrafo()
	if err := novo.AbrirBackup("local", caminho); err != nil {
		t.Fatal(err)
	}
	ler()
	s, err := novo.Autenticar(protocolo.Credenciais{Email: "p@teste.com", Senha: "senha1234"})
	if err != nil {
		t.Fatal(err)
	}
	reservas, err := novo.ConsultarReservas(s.ID)
	if err != nil || len(reservas) != 1 || reservas[0].Status != protocolo.Ativa {
		t.Fatalf("reserva perdida: %+v %v", reservas, err)
	}
	// Um hash novo danificado nao pode ser convertido como se fosse senha antiga.
	conta.Senha = "pbkdf2-sha256$600000$zz$00"
	estado.Usuarios["p@teste.com"] = conta
	gravar()
	if err := NovoGrafo().AbrirBackup("local", caminho); err == nil {
		t.Fatal("hash danificado aceito")
	}
}
