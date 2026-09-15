package testes

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"vaijunto/internal/protocolo"
	"vaijunto/internal/servidor"
)

type ambiente struct {
	g               *servidor.GrafoItinerarios
	motorista       string
	outroMotorista  string
	passageiro      string
	outroPassageiro string
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
	return s.Token
}

func preparar(t testing.TB) ambiente {
	t.Helper()
	g := servidor.NovoGrafo()
	return ambiente{g: g, motorista: usuario(t, g, "m@teste.com", protocolo.Motorista), outroMotorista: usuario(t, g, "m2@teste.com", protocolo.Motorista), passageiro: usuario(t, g, "p@teste.com", protocolo.Passageiro), outroPassageiro: usuario(t, g, "p2@teste.com", protocolo.Passageiro)}
}

func oferta(chave string, rota []string, hora string, vagas int) protocolo.PublicacaoCarona {
	trechos := make([]protocolo.OfertaTrecho, len(rota)-1)
	for i := range trechos {
		trechos[i] = protocolo.OfertaTrecho{DistanciaKM: 10, TempoViagem: 60, TempoParada: 15}
	}
	return protocolo.PublicacaoCarona{ValorKM: 1, Chave: chave, Rota: rota, DataHora: hora, Assentos: vagas, Trechos: trechos}
}

func publicar(t testing.TB, a ambiente, token, chave string, rota []string, hora string, vagas int) protocolo.Carona {
	t.Helper()
	c, err := a.g.Publicar(token, oferta(chave, rota, hora, vagas))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func ids(c protocolo.Carona) []string {
	r := make([]string, len(c.Trechos))
	for i, t := range c.Trechos {
		r[i] = t.Trecho.ID
	}
	return r
}

func TestAutenticacaoEAutorizacao(t *testing.T) {
	a := preparar(t)
	if _, err := a.g.Autenticar(protocolo.Credenciais{Email: "p@teste.com", Senha: "errada"}); err == nil {
		t.Fatal("senha incorreta aceita")
	}
	if _, err := a.g.ConsultarReservas(a.motorista); err == nil {
		t.Fatal("perfil incorreto aceito")
	}
	if _, err := a.g.Publicar(a.passageiro, oferta("x", []string{"A", "B"}, "2099-10-01T08:00:00-03:00", 1)); err == nil {
		t.Fatal("passageiro publicou")
	}
	if _, err := a.g.ConsultarCaronas("invalido"); err == nil {
		t.Fatal("token inválido aceito")
	}
	if err := a.g.Desconectar(a.passageiro); err != nil {
		t.Fatal(err)
	}
	if _, err := a.g.ConsultarReservas(a.passageiro); err == nil {
		t.Fatal("sessão revogada aceita")
	}
}

func TestMultigrafoDataConexoesEOrdenacao(t *testing.T) {
	a := preparar(t)
	publicar(t, a, a.motorista, "ab", []string{"A", "B"}, "2099-10-01T08:00:00-03:00", 2)
	publicar(t, a, a.outroMotorista, "bc", []string{"B", "C"}, "2099-10-01T09:15:00-03:00", 2)
	publicar(t, a, a.motorista, "ab2", []string{"A", "B"}, "2099-10-01T08:00:00-03:00", 2)
	publicar(t, a, a.outroMotorista, "cedo", []string{"B", "C"}, "2099-10-01T09:14:00-03:00", 2)
	publicar(t, a, a.motorista, "amanha", []string{"A", "C"}, "2099-10-02T08:00:00-03:00", 2)
	publicar(t, a, a.outroMotorista, "ciclo", []string{"B", "A"}, "2099-10-01T09:15:00-03:00", 2)
	direto := oferta("direto", []string{"A", "C"}, "2099-10-01T08:00:00-03:00", 2)
	direto.Trechos[0].DistanciaKM = 30
	if _, err := a.g.Publicar(a.motorista, direto); err != nil {
		t.Fatal(err)
	}
	for _, criterio := range []string{"PRECO", "TEMPO", "TRECHOS"} {
		r, err := a.g.Buscar(a.passageiro, protocolo.BuscaItinerario{Origem: " a ", Destino: "C", Data: "2099-10-01", OrdenarPor: criterio})
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Itinerarios) != 3 {
			t.Fatalf("caminhos: %+v", r)
		}
		if criterio == "PRECO" {
			if r.Itinerarios[0].PrecoTotal != 20 || r.Itinerarios[0].DuracaoTotal != 135 {
				t.Fatalf("pesos: %+v", r.Itinerarios[0])
			}
		} else if len(r.Itinerarios[0].Trechos) != 1 {
			t.Fatal("ordenação incorreta")
		}
	}
}

func TestConcorrenciaUltimoAssento(t *testing.T) {
	a := preparar(t)
	c := publicar(t, a, a.motorista, "abc", []string{"A", "B", "C"}, "2099-10-01T08:00:00-03:00", 1)
	var sucessos atomic.Int32
	var wg sync.WaitGroup
	inicio := make(chan struct{})
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-inicio
			token := a.passageiro
			if i%2 == 1 {
				token = a.outroPassageiro
			}
			if _, err := a.g.Confirmar(token, protocolo.ReservaItinerario{Chave: fmt.Sprint(i), TrechosIDs: ids(c)}); err == nil {
				sucessos.Add(1)
			}
		}()
	}
	close(inicio)
	wg.Wait()
	if sucessos.Load() != 1 {
		t.Fatalf("reservas confirmadas: %d", sucessos.Load())
	}
	caronas, err := a.g.ConsultarCaronas(a.motorista)
	if err != nil {
		t.Fatal(err)
	}
	for _, trecho := range caronas[0].Trechos {
		if trecho.Trecho.Assentos != 0 || len(trecho.Passageiros) != 1 || trecho.Passageiros[0].Assento != 1 {
			t.Fatalf("overbooking: %+v", trecho)
		}
	}
}

func TestAtomicidadeEAssentoPorTrecho(t *testing.T) {
	a := preparar(t)
	c := publicar(t, a, a.motorista, "abc", []string{"A", "B", "C"}, "2099-10-01T08:00:00-03:00", 1)
	caminho := ids(c)
	for i, invalida := range [][]string{nil, {caminho[0], caminho[0]}, {caminho[0], "inexistente"}, {caminho[1], caminho[0]}} {
		if _, err := a.g.Confirmar(a.passageiro, protocolo.ReservaItinerario{Chave: fmt.Sprint(i), TrechosIDs: invalida}); err == nil {
			t.Fatalf("aceitou %v", invalida)
		}
	}
	if _, err := a.g.Confirmar(a.outroPassageiro, protocolo.ReservaItinerario{Chave: "bc", TrechosIDs: caminho[1:]}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.g.Confirmar(a.passageiro, protocolo.ReservaItinerario{Chave: "abc", TrechosIDs: caminho}); err == nil {
		t.Fatal("reserva parcial aceita")
	}
	if _, err := a.g.Confirmar(a.passageiro, protocolo.ReservaItinerario{Chave: "ab", TrechosIDs: caminho[:1]}); err != nil {
		t.Fatalf("primeiro trecho indevidamente bloqueado: %v", err)
	}
}

func TestIdempotenciaECancelamento(t *testing.T) {
	a := preparar(t)
	c := publicar(t, a, a.motorista, "abc", []string{"A", "B", "C"}, "2099-10-01T08:00:00-03:00", 2)
	repetida := publicar(t, a, a.motorista, "abc", []string{"A", "B", "C"}, "2099-10-01T08:00:00-03:00", 2)
	if c.ID != repetida.ID {
		t.Fatal("publicação duplicada")
	}
	pedido := protocolo.ReservaItinerario{Chave: "mesma", TrechosIDs: ids(c)}
	r, err := a.g.Confirmar(a.passageiro, pedido)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := a.g.Confirmar(a.passageiro, pedido)
	if err != nil || r2.ID != r.ID {
		t.Fatal("reserva duplicada")
	}
	pedido.TrechosIDs = pedido.TrechosIDs[:1]
	if _, err := a.g.Confirmar(a.passageiro, pedido); err == nil {
		t.Fatal("chave reutilizada com outro caminho")
	}
	if _, err := a.g.CancelarReserva(a.outroPassageiro, r.ID); err == nil {
		t.Fatal("terceiro cancelou reserva")
	}
	for i := 0; i < 2; i++ {
		if _, err := a.g.CancelarReserva(a.passageiro, r.ID); err != nil {
			t.Fatal(err)
		}
	}
	caronas, _ := a.g.ConsultarCaronas(a.motorista)
	for _, trecho := range caronas[0].Trechos {
		if trecho.Trecho.Assentos != 2 || len(trecho.Passageiros) != 0 {
			t.Fatal("cancelamento duplicou ou não devolveu vagas")
		}
	}
	reservas, _ := a.g.ConsultarReservas(a.passageiro)
	if len(reservas) != 1 || reservas[0].Status != protocolo.Cancelada {
		t.Fatal("histórico ausente")
	}
	r2, err = a.g.Confirmar(a.passageiro, protocolo.ReservaItinerario{Chave: "mesma", TrechosIDs: ids(c)})
	if err != nil || r2.Status != protocolo.Cancelada {
		t.Fatal("repetição reativou reserva cancelada")
	}
}

func TestCancelamentoCaronaReverteItinerarioInteiro(t *testing.T) {
	a := preparar(t)
	ab := publicar(t, a, a.motorista, "ab", []string{"A", "B"}, "2099-10-01T08:00:00-03:00", 1)
	bc := publicar(t, a, a.outroMotorista, "bc", []string{"B", "C"}, "2099-10-01T09:15:00-03:00", 1)
	if _, err := a.g.Confirmar(a.passageiro, protocolo.ReservaItinerario{Chave: "abc", TrechosIDs: append(ids(ab), ids(bc)...)}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.g.CancelarCarona(a.outroMotorista, ab.ID); err == nil {
		t.Fatal("terceiro cancelou carona")
	}
	if _, err := a.g.CancelarCarona(a.motorista, ab.ID); err != nil {
		t.Fatal(err)
	}
	restantes, _ := a.g.ConsultarCaronas(a.outroMotorista)
	if restantes[0].Trechos[0].Trecho.Assentos != 1 {
		t.Fatal("vaga do outro motorista não devolvida")
	}
	reservas, _ := a.g.ConsultarReservas(a.passageiro)
	if reservas[0].Status != protocolo.Cancelada {
		t.Fatal("itinerário parcial mantido")
	}
	busca, err := a.g.Buscar(a.passageiro, protocolo.BuscaItinerario{Origem: "A", Destino: "B", Data: "2099-10-01"})
	if err != nil || len(busca.Itinerarios) != 0 {
		t.Fatal("carona cancelada apareceu na busca")
	}
}

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
