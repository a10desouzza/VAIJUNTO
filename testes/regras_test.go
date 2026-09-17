// testes/regras_test.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Testes de autorizacao, busca, validacao e isolamento do estado.

package testes

import (
	"encoding/json"
	"testing"
	"vaijunto/internal/protocolo"
	"vaijunto/internal/servidor"
)

// TestMultigrafoDataConexoesEOrdenacao: Verifica ofertas paralelas, filtro por data,
// compatibilidade das conexoes e ordenacao dos itinerarios.
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
	// Partidas iguais: o preco e o primeiro desempate.
	if r.Itinerarios[0].PrecoTotal != 20 || r.Itinerarios[0].DuracaoTotal != 135 {
		t.Fatalf("pesos: %+v", r.Itinerarios[0])
	}
}

// TestRoteadorRejeitaMensagensInvalidas: Envia mensagens malformadas ou incompativeis e verifica
// respostas de erro do roteador.
func TestRoteadorRejeitaMensagensInvalidas(t *testing.T) {
	g := servidor.NovoGrafo()
	for _, msg := range []string{`{`, `null`, `[]`, `{}`, `{"acao":"X","dados":{}}`, `{"acao":"CADASTRAR","dados":null}`, `{"acao":"AUTENTICAR","dados":{"email":"x","extra":1}}`, `{"acao":"AUTENTICAR","acao":"CADASTRAR","dados":{}}`, `{"acao":"CADASTRAR","dados":{"email":"a","email":"b"}}`, `{"acao":"DESCONECTAR","dados":{}}`} {
		raw := servidor.ProcessarMensagem([]byte(msg), g)
		var r protocolo.Resposta
		if err := json.Unmarshal(raw, &r); err != nil {
			t.Fatal(err)
		}
		if r.Status != protocolo.Erro || r.Mensagem == "" || raw[len(raw)-1] != '\n' {
			t.Fatalf("resposta inválida: %s", raw)
		}
	}
}

// TestPublicacaoInvalidaNaoCriaTrechos: Tenta publicar dados invalidos e verifica que nao foram
// criados trechos parciais.
func TestPublicacaoInvalidaNaoCriaTrechos(t *testing.T) {
	a := preparar(t)
	for _, alterar := range []func(*protocolo.PublicacaoCarona){
		func(p *protocolo.PublicacaoCarona) { p.Rota = []string{"A", "A"} },
		func(p *protocolo.PublicacaoCarona) { p.Assentos = 0 },
		func(p *protocolo.PublicacaoCarona) { p.Trechos[0].DistanciaKM = -1 },
		func(p *protocolo.PublicacaoCarona) { p.Trechos[0].DistanciaKM = 0.001 },
		func(p *protocolo.PublicacaoCarona) { p.DataHora = "amanhã" },
		func(p *protocolo.PublicacaoCarona) { p.Trechos = nil },
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

// TestCopiasNaoExpoemEstado: Altera dados devolvidos nas respostas e verifica que as copias nao
// permitem modificar o estado central.
func TestCopiasNaoExpoemEstado(t *testing.T) {
	a := preparar(t)
	c := publicar(t, a, a.motorista, "ab", []string{"A", "B"}, "2099-10-01T08:00:00-03:00", 2)
	r, err := a.g.Confirmar(a.passageiro, protocolo.ReservaItinerario{Chave: "x", TrechosIDs: ids(c)})
	if err != nil {
		t.Fatal(err)
	}
	r.Assentos[0].Numero = 99
	c.Rota[0] = "ALTERADA"
	reservas, _ := a.g.ConsultarReservas(a.passageiro)
	caronas, _ := a.g.ConsultarCaronas(a.motorista)
	if reservas[0].Assentos[0].Numero != 1 || caronas[0].Rota[0] != "A" {
		t.Fatal("estado interno exposto")
	}
}
