// Arquivo: testes/rest_test.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Testa a consulta entre empresas sem misturar suas bases locais.
// Verifica autenticação, identidade dos catálogos e agrupamento dos trechos por dono.

package testes

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"vaijunto/internal/configuracao"
	"vaijunto/internal/consorcio"
	"vaijunto/internal/protocolo"
	"vaijunto/internal/servidor"
)

const tokenTeste = "token-local-testes-123456"

// grafoPublicado cria um grafo de teste e publica uma oferta entre as cidades recebidas.
// Recebe: t *testing.T, origem, destino string.
// Retorna: *servidor.GrafoItinerarios.
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

// novaTeste cria a API de uma empresa de teste e interrompe o teste se a configuracao falhar.
// Recebe: t *testing.T, id string, g consorcio.Fonte, pares []configuracao.Empresa.
// Retorna: *consorcio.API.
func novaTeste(t *testing.T, id string, g consorcio.Fonte, pares []configuracao.Empresa) *consorcio.API {
	t.Helper()
	a, err := consorcio.NovaAPI(id, tokenTeste, pares, g, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// TestTresEmpresasConsultamSemMisturarArmazenamento verifica o cenario: Tres Empresas Consultam Sem Misturar Armazenamento.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
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
		// Os IDs locais coincidem; empresa_id preserva a identidade de cada recurso.
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

// TestRESTExigeTokenERejeitaIdentidadeDivergente verifica o cenario: RESTExige Token ERejeita Identidade Divergente.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestRESTExigeTokenERejeitaIdentidadeDivergente(t *testing.T) {
	g := servidor.NovoGrafo()
	api := novaTeste(t, "empresa-a", g, nil)
	for _, path := range []string{"/saude", "/trechos", "/consorcio/trechos"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		api.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatal("endpoint sem autenticacao")
		}
	}
	b := httptest.NewServer(novaTeste(t, "empresa-c", g, nil).Handler())
	defer b.Close()
	api = novaTeste(t, "empresa-a", g, []configuracao.Empresa{{ID: "empresa-b", URL: b.URL}})
	r := api.ConsultarTodos(context.Background())
	if len(r.Indisponiveis) != 1 {
		t.Fatal("identidade divergente aceita")
	}
}

// TestAgruparTrechosIdentificaEmpresaEPreservaOrdemLocal verifica o cenario: Agrupar Trechos Identifica Empresa EPreserva Ordem Local.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestAgruparTrechosIdentificaEmpresaEPreservaOrdemLocal(t *testing.T) {
	p := protocolo.ReservaDistribuida{Chave: "viagem", Trechos: []protocolo.ReferenciaTrecho{{EmpresaID: "empresa-a", TrechoID: "T1"}, {EmpresaID: "empresa-b", TrechoID: "T1"}, {EmpresaID: "empresa-a", TrechoID: "T2"}}}
	g, err := consorcio.AgruparTrechos(p)
	if err != nil {
		t.Fatal(err)
	}
	esperado := map[string][]string{"empresa-a": {"T1", "T2"}, "empresa-b": {"T1"}}
	if !reflect.DeepEqual(g, esperado) {
		t.Fatalf("agrupamento incorreto: %+v", g)
	}
	p.Trechos = append(p.Trechos, p.Trechos[0])
	if _, err := consorcio.AgruparTrechos(p); err == nil {
		t.Fatal("mesma referencia repetida aceita")
	}
}
