// Autor: Arthur de Andrade Souza.
// Matéria: TEC502 - TP01 - Concorrência e Conectividade - 2026.2.
// Declaração de não plágio: Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
// Funcionalidade: Testa senhas protegidas, conversão de senhas legadas e avanço concorrente do relógio de Lamport.
package servidor

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"vaijunto/internal/protocolo"
)

// TestLamportMaximoEConcorrencia verifica max(local, recebido)+1 e ausência de perda/repetição em eventos concorrentes.
// Recebe: controle de teste t.
func TestLamportMaximoEConcorrencia(t *testing.T) {
	d := &Distribuido{}
	if d.evento(0) != 1 || d.evento(100) != 101 || d.evento(2) != 102 {
		t.Fatal("regra max(local, recebido)+1 incorreta")
	}
	var wg sync.WaitGroup
	marcas := make(chan uint64, 100)
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			marcas <- d.evento(0)
		}()
	}
	wg.Wait()
	close(marcas)
	vistos := make(map[uint64]bool)
	for marca := range marcas {
		if marca <= 102 || vistos[marca] {
			t.Fatalf("evento concorrente repetiu ou retrocedeu: %d", marca)
		}
		vistos[marca] = true
	}
	if d.relogio != 202 {
		t.Fatal("eventos concorrentes foram perdidos")
	}
}

// TestSenhasComSaltEComparacao verifica salt individual, hash de senha e comparação das credenciais.
// Recebe: controle de teste t.
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

// TestBackupMigraSenhasLegadasSemPerderReservas verifica conversão de senha legada e preservação das reservas no backup.
// Recebe: controle de teste t.
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

	conta.Senha = "pbkdf2-sha256$600000$zz$00"
	estado.Usuarios["p@teste.com"] = conta
	gravar()
	if err := NovoGrafo().AbrirBackup("local", caminho); err == nil {
		t.Fatal("hash danificado aceito")
	}
}
