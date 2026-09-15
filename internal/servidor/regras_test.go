package servidor

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
	"vaijunto/internal/protocolo"
)

type cenarioViagem struct {
	g     *GrafoItinerarios
	agora time.Time
	m1    string
	m2    string
	p     string
	outro string
	ab    protocolo.Carona
	bc    protocolo.Carona
}

func criarCenario(t *testing.T) *cenarioViagem {
	t.Helper()
	c := &cenarioViagem{g: NovoGrafo(), agora: time.Date(2099, 10, 1, 8, 0, 0, 0, time.FixedZone("Bahia", -3*3600))}
	c.g.agora = func() time.Time { return c.agora }
	usuario := func(email, perfil string) string {
		t.Helper()
		if _, err := c.g.Cadastrar(protocolo.Cadastro{Nome: "Pessoa Teste", Email: email, Senha: "senha1234", Perfil: perfil}); err != nil {
			t.Fatal(err)
		}
		s, err := c.g.Autenticar(protocolo.Credenciais{Email: email, Senha: "senha1234"})
		if err != nil {
			t.Fatal(err)
		}
		return s.Token
	}
	c.m1 = usuario("m1@teste.com", protocolo.Motorista)
	c.m2 = usuario("m2@teste.com", protocolo.Motorista)
	c.p = usuario("p@teste.com", protocolo.Passageiro)
	c.outro = usuario("outro@teste.com", protocolo.Passageiro)
	publicar := func(token, chave string, rota []string, minutos int) protocolo.Carona {
		t.Helper()
		ofertas := make([]protocolo.OfertaTrecho, len(rota)-1)
		for i := range ofertas {
			ofertas[i] = protocolo.OfertaTrecho{DistanciaKM: 10, TempoViagem: 10, TempoParada: 5}
		}
		carona, err := c.g.Publicar(token, protocolo.PublicacaoCarona{Chave: chave, Rota: rota, DataHora: c.agora.Add(time.Duration(minutos) * time.Minute).Format(time.RFC3339), ValorKM: 2, Assentos: 2, Trechos: ofertas})
		if err != nil {
			t.Fatal(err)
		}
		return carona
	}
	c.ab = publicar(c.m1, "ab", []string{"A", "B"}, 30)
	c.bc = publicar(c.m2, "bcd", []string{"B", "C", "D"}, 50)
	return c
}

func (c *cenarioViagem) reservar(t *testing.T) protocolo.Reserva {
	t.Helper()
	r, err := c.g.Confirmar(c.p, protocolo.ReservaItinerario{Chave: "abc", TrechosIDs: []string{c.ab.Trechos[0].Trecho.ID, c.bc.Trechos[0].Trecho.ID}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNaoCancelaConexaoDeItinerarioIniciado(t *testing.T) {
	for _, tipo := range []string{"trecho", "carona"} {
		for _, minutos := range []int{30, 31} {
			t.Run(fmt.Sprintf("%s_%d", tipo, minutos), func(t *testing.T) {
				c := criarCenario(t)
				r := c.reservar(t)
				c.agora = c.agora.Add(time.Duration(minutos) * time.Minute)
				var err error
				if tipo == "trecho" {
					_, err = c.g.CancelarTrecho(c.m2, c.bc.Trechos[0].Trecho.ID)
				} else {
					_, err = c.g.CancelarCarona(c.m2, c.bc.ID)
				}
				if err == nil || !strings.Contains(err.Error(), "já iniciou o itinerário") {
					t.Fatalf("cancelamento deveria ser bloqueado: %v", err)
				}
				if c.g.reservas[r.ID].Status != protocolo.Ativa {
					t.Fatal("reserva foi alterada")
				}
				for _, a := range r.Assentos {
					if c.g.trechos[a.Trecho.ID].Assentos != 1 || c.g.trechos[a.Trecho.ID].Status != protocolo.Ativa {
						t.Fatal("vaga ou trecho alterado")
					}
				}
				if c.g.caronas[c.bc.ID].status != protocolo.Ativa || len(c.g.notificacoes[r.Passageiro]) != 0 {
					t.Fatal("cancelamento recusado teve efeito")
				}
			})
		}
	}
}

func TestCancelamentoAntesDaPartidaENotificacoes(t *testing.T) {
	c := criarCenario(t)
	r := c.reservar(t)
	c.agora = c.agora.Add(29*time.Minute + 59*time.Second)
	id := c.bc.Trechos[0].Trecho.ID
	if _, err := c.g.CancelarTrecho(c.m1, id); err == nil {
		t.Fatal("outro motorista cancelou")
	}
	for i := 0; i < 2; i++ {
		if _, err := c.g.CancelarTrecho(c.m2, id); err != nil {
			t.Fatal(err)
		}
	}
	if c.g.reservas[r.ID].Status != protocolo.Cancelada {
		t.Fatal("reserva não cancelada")
	}
	for _, a := range r.Assentos {
		if c.g.trechos[a.Trecho.ID].Assentos != 2 {
			t.Fatal("vaga não devolvida ou devolvida duas vezes")
		}
	}
	if c.g.caronas[c.bc.ID].status != protocolo.Parcial {
		t.Fatal("carona deveria ser parcial")
	}
	if c.g.trechos[c.bc.Trechos[1].Trecho.ID].Status != protocolo.Ativa {
		t.Fatal("trecho independente cancelado")
	}
	avisos, err := c.g.ConsultarNotificacoes(c.p)
	if err != nil || len(avisos) != 1 || avisos[0].ReservaID != r.ID || avisos[0].Lida {
		t.Fatalf("notificações: %+v %v", avisos, err)
	}
	if err := c.g.LerNotificacao(c.outro, avisos[0].ID); err == nil {
		t.Fatal("outro usuário leu aviso")
	}
	if err := c.g.LerNotificacao(c.p, avisos[0].ID); err != nil {
		t.Fatal(err)
	}
	avisos, _ = c.g.ConsultarNotificacoes(c.p)
	if !avisos[0].Lida {
		t.Fatal("aviso não marcado")
	}
	resultado, err := c.g.Buscar(c.outro, protocolo.BuscaItinerario{Origem: "C", Destino: "D", Data: "2099-10-01"})
	if err != nil || len(resultado.Itinerarios) != 1 {
		t.Fatalf("trecho não cancelado indisponível: %+v %v", resultado, err)
	}
}

func TestCaronaIniciadaBloqueiaCancelamentoEReserva(t *testing.T) {
	c := criarCenario(t)
	c.agora = c.agora.Add(50 * time.Minute)
	if _, err := c.g.CancelarCarona(c.m2, c.bc.ID); err == nil {
		t.Fatal("carona iniciada cancelada")
	}
	tid := c.bc.Trechos[1].Trecho.ID
	if _, err := c.g.CancelarTrecho(c.m2, tid); err == nil {
		t.Fatal("trecho futuro de carona iniciada cancelado")
	}
	if _, err := c.g.Confirmar(c.p, protocolo.ReservaItinerario{Chave: "tarde", TrechosIDs: []string{tid}}); err == nil {
		t.Fatal("reservou durante a carona")
	}
	resultado, err := c.g.Buscar(c.p, protocolo.BuscaItinerario{Origem: "C", Destino: "D", Data: "2099-10-01"})
	if err != nil || len(resultado.Itinerarios) != 0 {
		t.Fatal("busca retornou carona iniciada")
	}
}

func TestItinerarioIniciadoNaoBloqueiaTrechoSemRelacao(t *testing.T) {
	c := criarCenario(t)
	r := c.reservar(t)
	c.agora = c.agora.Add(31 * time.Minute)
	if _, err := c.g.CancelarTrecho(c.m2, c.bc.Trechos[1].Trecho.ID); err != nil {
		t.Fatal(err)
	}
	if c.g.reservas[r.ID].Status != protocolo.Ativa {
		t.Fatal("reserva independente cancelada")
	}
}

func TestPrecoKMEMesmaRota(t *testing.T) {
	c := criarCenario(t)
	p := protocolo.PublicacaoCarona{Chave: "nova", Rota: []string{"A", "B"}, DataHora: c.ab.DataHora, ValorKM: 1.25, Assentos: 2, Trechos: []protocolo.OfertaTrecho{{DistanciaKM: 10.25, TempoViagem: 10}}}
	a, err := c.g.Publicar(c.m1, p)
	if err != nil {
		t.Fatal(err)
	}
	if a.Trechos[0].Trecho.Preco != 12.81 {
		t.Fatalf("preço incorreto: %v", a.Trechos[0].Trecho.Preco)
	}
	p.Chave = "outra"
	b, err := c.g.Publicar(c.m1, p)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID || a.Trechos[0].Trecho.ID == b.Trechos[0].Trecho.ID {
		t.Fatal("IDs iguais para publicações distintas")
	}
	repetida, err := c.g.Publicar(c.m1, p)
	if err != nil || repetida.ID != b.ID {
		t.Fatal("retry duplicou carona")
	}
	busca, err := c.g.Buscar(c.p, protocolo.BuscaItinerario{Origem: "A", Destino: "B", Data: "2099-10-01"})
	if err != nil || len(busca.Itinerarios) != 3 {
		t.Fatalf("arestas paralelas perdidas: %+v %v", busca, err)
	}
}

func TestCancelamentoEConfirmacaoConcorrentes(t *testing.T) {
	c := criarCenario(t)
	inicio := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-inicio
			if i%2 == 0 {
				c.g.Confirmar(c.p, protocolo.ReservaItinerario{Chave: fmt.Sprint(i), TrechosIDs: []string{c.ab.Trechos[0].Trecho.ID, c.bc.Trechos[0].Trecho.ID}})
			} else {
				c.g.CancelarTrecho(c.m2, c.bc.Trechos[0].Trecho.ID)
			}
		}()
	}
	close(inicio)
	wg.Wait()
	for _, id := range []string{c.ab.Trechos[0].Trecho.ID, c.bc.Trechos[0].Trecho.ID} {
		if c.g.trechos[id].Assentos != 2 || len(c.g.ocupados[id]) != 0 {
			t.Fatal("vagas inconsistentes após disputa")
		}
	}
	for _, r := range c.g.reservas {
		if r.Status != protocolo.Cancelada {
			t.Fatal("reserva ativa em trecho cancelado")
		}
	}
}

func TestNovasAcoesTCPProtocoladas(t *testing.T) {
	c := criarCenario(t)
	r := c.reservar(t)
	tid, _ := json.Marshal(protocolo.Identificador{ID: c.bc.Trechos[0].Trecho.ID})
	for _, req := range []protocolo.Requisicao{
		{Acao: protocolo.AcaoCancelarTrecho, Token: c.m2, Dados: tid},
		{Acao: protocolo.AcaoNotificacoes, Token: c.p, Dados: json.RawMessage(`{}`)},
	} {
		raw, _ := json.Marshal(req)
		var resp protocolo.Resposta
		if err := json.Unmarshal(ProcessarMensagem(raw, c.g), &resp); err != nil || resp.Status != protocolo.Sucesso {
			t.Fatalf("resposta: %+v %v", resp, err)
		}
	}
	if c.g.reservas[r.ID].Status != protocolo.Cancelada {
		t.Fatal("roteador não cancelou")
	}
}
