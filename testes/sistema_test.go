// TEC502 - Problema 2. Autor: Arthur de Andrade Souza.
// Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
package testes

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"vaijunto/internal/cliente"
	"vaijunto/internal/configuracao"
	"vaijunto/internal/consorcio"
	"vaijunto/internal/protocolo"
	"vaijunto/internal/servidor"
)

func TestTresEmpresasConsultamSemMisturarArmazenamento(t *testing.T) {
	ga := grafoPublicado(t, "A", "B")
	gb := grafoPublicado(t, "B", "C")
	gc := grafoPublicado(t, "C", "D")
	b := httptest.NewServer(novaTeste(t, "empresa-b", gb, nil).Handler())
	defer b.Close()
	c := httptest.NewServer(novaTeste(t, "empresa-c", gc, nil).Handler())
	defer c.Close()
	api := novaTeste(t, "empresa-a", ga, []configuracao.Empresa{{ID: "empresa-b", URL: b.URL}, {ID: "empresa-c", URL: c.URL}})
	a := httptest.NewServer(api.Handler())
	defer a.Close()
	req, _ := http.NewRequest(http.MethodGet, a.URL+"/consorcio/trechos", nil)
	req.Header.Set("Authorization", "Bearer "+tokenTeste)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	bts, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("HTTP %d: %s", resp.StatusCode, bts)
	}
	if strings.Contains(string(bts), "privado@") || strings.Contains(string(bts), "motorista_email") {
		t.Fatal("dados pessoais exportados")
	}
	var resultado consorcio.Consulta
	if err := json.Unmarshal(bts, &resultado); err != nil {
		t.Fatal(err)
	}
	if len(resultado.Trechos) != 3 || len(resultado.Indisponiveis) != 0 {
		t.Fatalf("resultado: %+v", resultado)
	}
	for i, id := range []string{"empresa-a", "empresa-b", "empresa-c"} {
		if resultado.Trechos[i].EmpresaID != id {
			t.Fatal("dono do trecho incorreto")
		}
		if resultado.Trechos[i].ID != resultado.Trechos[0].ID {
			t.Fatal("cenario deveria usar IDs locais iguais")
		}
	}
	if len(ga.TrechosDisponiveis()) != 1 || len(gb.TrechosDisponiveis()) != 1 || len(gc.TrechosDisponiveis()) != 1 {
		t.Fatal("consulta alterou armazenamento local")
	}
	b.Close()
	parcial := api.ConsultarTodos(context.Background())
	if len(parcial.Trechos) != 2 || len(parcial.Indisponiveis) != 1 || parcial.Indisponiveis[0] != "empresa-b" {
		t.Fatalf("falha nao isolada: %+v", parcial)
	}
}

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

func TestPublicacaoInvalidaNaoCriaTrechos(t *testing.T) {
	a := preparar(t)
	for _, alterar := range []func(*protocolo.PublicacaoCarona){
		func(p *protocolo.PublicacaoCarona) {
			p.Rota = []string{"A", "A"}
		},
		func(p *protocolo.PublicacaoCarona) {
			p.Assentos = 0
		},
		func(p *protocolo.PublicacaoCarona) {
			p.Trechos[0].DistanciaKM = -1
		},
		func(p *protocolo.PublicacaoCarona) {
			p.Trechos[0].DistanciaKM = 0.001
		},
		func(p *protocolo.PublicacaoCarona) {
			p.DataHora = "amanhã"
		},
		func(p *protocolo.PublicacaoCarona) {
			p.Trechos = nil
		},
	} {
		p := oferta("x", []string{"A", "B"}, "2099-10-01T08:00:00-03:00", 2)
		alterar(&p)
		if _, err := a.g.Publicar(a.motorista, p); err == nil {
			t.Fatal("publicação inválida aceita")
		}
	}
	caronas, _ := a.g.ConsultarCaronas(a.motorista)
	if len(caronas) != 0 {
		t.Fatal("publicação parcial")
	}
}

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

func TestMultigrafoDataConexoesEOrdenacao(t *testing.T) {
	a := preparar(t)
	publicar(t, a, a.motorista, "ab", []string{"A", "B"}, "2099-10-01T08:00:00-03:00", 2)
	publicar(t, a, a.outroMotorista, "bc", []string{"B", "C"}, "2099-10-01T09:15:00-03:00", 2)
	publicar(t, a, a.motorista, "ab2", []string{"A", "B"}, "2099-10-01T08:00:00-03:00", 2)
	publicar(t, a, a.outroMotorista, "cedo", []string{"B", "C"}, "2099-10-01T08:59:00-03:00", 2)
	publicar(t, a, a.motorista, "amanha", []string{"A", "C"}, "2099-10-02T08:00:00-03:00", 2)
	publicar(t, a, a.outroMotorista, "ciclo", []string{"B", "A"}, "2099-10-01T09:15:00-03:00", 2)
	direto := oferta("direto", []string{"A", "C"}, "2099-10-01T08:00:00-03:00", 2)
	direto.Trechos[0].Preco = 30
	if _, err := a.g.Publicar(a.motorista, direto); err != nil {
		t.Fatal(err)
	}
	r, err := a.g.Buscar(a.passageiro, protocolo.BuscaItinerario{Origem: " a ", Destino: "C", Data: "2099-10-01"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Itinerarios) != 3 {
		t.Fatalf("caminhos: %+v", r)
	}
	if r.Itinerarios[0].PrecoTotal != 20 || r.Itinerarios[0].DuracaoTotal != 135 {
		t.Fatalf("pesos: %+v", r.Itinerarios[0])
	}
}

func grafoPublicado(t *testing.T, origem, destino string) *servidor.GrafoItinerarios {
	t.Helper()
	g := servidor.NovoGrafo()
	_, err := g.Cadastrar(protocolo.Cadastro{Nome: "Motorista Teste", Email: "privado@teste.com", Senha: "senha1234", Perfil: protocolo.Motorista})
	if err != nil {
		t.Fatal(err)
	}
	s, err := g.Autenticar(protocolo.Credenciais{Email: "privado@teste.com", Senha: "senha1234"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = g.Publicar(s.ID, protocolo.PublicacaoCarona{Chave: "oferta", Rota: []string{origem, destino}, DataHora: "2099-10-01T08:00:00-03:00", Assentos: 2, Trechos: []protocolo.OfertaTrecho{{Preco: 20, DistanciaKM: 10, TempoViagem: 10}}})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestVoltarAbandonaCadastroSemRede(t *testing.T) {
	var saida bytes.Buffer
	err := cliente.ExecutarMenu("127.0.0.1:1", protocolo.Passageiro, strings.NewReader("2\nAna\n/voltar\n0\n"), &saida)
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

func usuario(t testing.TB, g *servidor.GrafoItinerarios, email, perfil string) string {
	t.Helper()
	if _, err := g.Cadastrar(protocolo.Cadastro{Nome: "Pessoa Teste", Email: email, Senha: "senha-teste-123", Perfil: perfil}); err != nil {
		t.Fatal(err)
	}
	s, err := g.Autenticar(protocolo.Credenciais{Email: email, Senha: "senha-teste-123"})
	if err != nil {
		t.Fatal(err)
	}
	return s.ID
}

func publicar(t testing.TB, a ambiente, sessaoID, chave string, rota []string, hora string, vagas int) protocolo.Carona {
	t.Helper()
	c, err := a.g.Publicar(sessaoID, oferta(chave, rota, hora, vagas))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

type ambiente struct {
	g               *servidor.GrafoItinerarios
	motorista       string
	outroMotorista  string
	passageiro      string
	outroPassageiro string
}

func ids(c protocolo.Carona) []string {
	r := make([]string, len(c.Trechos))
	for i, t := range c.Trechos {
		r[i] = t.Trecho.ID
	}
	return r
}
