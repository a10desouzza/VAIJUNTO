// internal/servidor/itinerarios_test.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Testes de publicacao, precos, conexoes e horarios dos itinerarios.

package servidor

import (
	"fmt"
	"testing"
	"time"
	"vaijunto/internal/protocolo"
)

// TestPublicacaoSemParadaNoDestinoFinal: Recebe o teste e publica rotas de varios tamanhos.
// Confere horarios, parada final zero e repeticao sem alterar a entrada. Sem retorno.
func TestPublicacaoSemParadaNoDestinoFinal(t *testing.T) {
	c := criarCenario(t)
	for _, n := range []int{2, 3, 8, 21} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			p := protocolo.PublicacaoCarona{Chave: fmt.Sprint("final-", n), Assentos: 2, DataHora: c.agora.Add(time.Hour).Format(time.RFC3339)}
			for i := 0; i < n; i++ {
				p.Rota = append(p.Rota, fmt.Sprintf("Cidade %c", 'A'+i))
				if i < n-1 {
					p.Trechos = append(p.Trechos, protocolo.OfertaTrecho{Preco: 20, DistanciaKM: 10, TempoViagem: 60, TempoParada: 15})
				}
			}
			carona, err := c.g.Publicar(c.m1, p)
			if err != nil {
				t.Fatal(err)
			}
			for i, item := range carona.Trechos {
				esperada := 15
				if i == n-2 {
					esperada = 0
				}
				if item.Trecho.TempoParada != esperada {
					t.Fatalf("trecho %d: parada %d, esperado %d", i, item.Trecho.TempoParada, esperada)
				}
				hora := c.agora.Add(time.Hour + time.Duration(i*75)*time.Minute).Format(time.RFC3339)
				if item.Trecho.DataHora != hora {
					t.Fatalf("horário: %s, esperado %s", item.Trecho.DataHora, hora)
				}
			}
			if p.Trechos[n-2].TempoParada != 15 {
				t.Fatal("alterou entrada do chamador")
			}
			p.Trechos[n-2].TempoParada = 0
			repetida, err := c.g.Publicar(c.m1, p)
			if err != nil || repetida.ID != carona.ID {
				t.Fatalf("repetição normalizada: %v, %s", err, repetida.ID)
			}
		})
	}
}

// TestConexaoNaChegadaAoDestinoFinal: Recebe o teste e cria uma conexao no instante da chegada.
// Confere busca e reserva sem espera no destino final. Sem retorno.
func TestConexaoNaChegadaAoDestinoFinal(t *testing.T) {
	c := criarCenario(t)
	// A-B chega às 08:40: a parada de 5 enviada pelo cliente antigo é ignorada.
	bc, err := c.g.Publicar(c.m2, protocolo.PublicacaoCarona{Chave: "conexao-imediata", Rota: []string{"B", "E"}, DataHora: c.agora.Add(40 * time.Minute).Format(time.RFC3339), Assentos: 1, Trechos: []protocolo.OfertaTrecho{{Preco: 10, DistanciaKM: 10, TempoViagem: 10, TempoParada: 100}}})
	if err != nil {
		t.Fatal(err)
	}
	busca, err := c.g.Buscar(c.p, protocolo.BuscaItinerario{Origem: "A", Destino: "E", Data: "2099-10-01"})
	if err != nil || len(busca.Itinerarios) != 1 {
		t.Fatalf("busca: %+v; %v", busca, err)
	}
	if busca.Itinerarios[0].DuracaoTotal != 20 {
		t.Fatalf("duração: %d", busca.Itinerarios[0].DuracaoTotal)
	}
	_, err = c.g.Confirmar(c.p, protocolo.ReservaItinerario{Chave: "sem-espera-final", TrechosIDs: []string{c.ab.Trechos[0].Trecho.ID, bc.Trechos[0].Trecho.ID}})
	if err != nil {
		t.Fatal(err)
	}
}

// TestPrecoPorTrechoEMesmaRota: Confere o preco escolhido, IDs diferentes para ofertas distintas e
// repeticao sem duplicar a publicacao.
func TestPrecoPorTrechoEMesmaRota(t *testing.T) {
	c := criarCenario(t)
	p := protocolo.PublicacaoCarona{Chave: "nova", Rota: []string{"A", "B"}, DataHora: c.ab.DataHora, Assentos: 2, Trechos: []protocolo.OfertaTrecho{{Preco: 12.81, DistanciaKM: 10.25, TempoViagem: 10}}}
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

// TestReservaSerrinhaFeiraSalvadorDuranteViagemEParada: Recebe o teste e avanca o relogio por viagem e parada.
// Confere a reserva do trecho futuro e sua recusa a partir da partida. Sem retorno.
func TestReservaSerrinhaFeiraSalvadorDuranteViagemEParada(t *testing.T) {
	for _, caso := range []struct {
		nome      string
		depois    time.Duration
		permitido bool
	}{
		{"partida de Serrinha", 0, true},
		{"a caminho de Feira", 30 * time.Minute, true},
		{"chegada em Feira", 60 * time.Minute, true},
		{"durante parada", 65 * time.Minute, true},
		{"ultimo segundo", 70*time.Minute - time.Second, true},
		{"partida de Feira", 70 * time.Minute, false},
		{"apos partida", 71 * time.Minute, false},
	} {
		t.Run(caso.nome, func(t *testing.T) {
			c := criarCenario(t)
			inicio := c.agora.Add(time.Minute)
			carona, err := c.g.Publicar(c.m1, protocolo.PublicacaoCarona{
				Chave: "serrinha", Rota: []string{"Serrinha", "Feira de Santana", "Salvador"}, DataHora: inicio.Format(time.RFC3339), Assentos: 1,
				Trechos: []protocolo.OfertaTrecho{{Preco: 17.35, DistanciaKM: 80, TempoViagem: 60, TempoParada: 10}, {Preco: 42.10, DistanciaKM: 110, TempoViagem: 90}},
			})
			if err != nil {
				t.Fatal(err)
			}
			c.agora = inicio.Add(caso.depois)
			c.reautenticar(t)
			resultado, err := c.g.Buscar(c.p, protocolo.BuscaItinerario{Origem: "Feira de Santana", Destino: "Salvador", Data: "2099-10-01"})
			if err != nil {
				t.Fatal(err)
			}
			if (len(resultado.Itinerarios) == 1) != caso.permitido {
				t.Fatalf("busca: %+v", resultado)
			}
			primeiro := carona.Trechos[0].Trecho.ID
			segundo := carona.Trechos[1].Trecho.ID
			if _, err := c.g.Confirmar(c.p, protocolo.ReservaItinerario{Chave: "iniciado", TrechosIDs: []string{primeiro, segundo}}); err == nil {
				t.Fatal("reservou trecho iniciado")
			}
			if c.g.trechos[segundo].Assentos != 1 {
				t.Fatal("falha parcial consumiu vaga futura")
			}
			r, err := c.g.Confirmar(c.p, protocolo.ReservaItinerario{Chave: "futuro", TrechosIDs: []string{segundo}})
			if (err == nil) != caso.permitido {
				t.Fatalf("reserva: %v", err)
			}
			if caso.permitido {
				if r.PrecoTotal != 42.10 || r.Assentos[0].Trecho.DataHora != inicio.Add(70*time.Minute).Format(time.RFC3339) {
					t.Fatalf("reserva: %+v", r)
				}
				if _, err := c.g.Confirmar(c.outro, protocolo.ReservaItinerario{Chave: "lotado", TrechosIDs: []string{segundo}}); err == nil {
					t.Fatal("reservou sem vagas")
				}
			}
		})
	}
}

// TestBuscaOrdenaPorPartidaMesmoComPrecoEDuracaoMaiores: Recebe o teste e publica horarios com fusos diferentes.
// Confere a ordem por partida, mesmo com precos e duracoes maiores. Sem retorno.
func TestBuscaOrdenaPorPartidaMesmoComPrecoEDuracaoMaiores(t *testing.T) {
	c := criarCenario(t)
	for i, p := range []struct {
		hora    string
		preco   float64
		duracao int
	}{
		{"2099-10-01T10:00:00-03:00", 1, 5},
		{"2099-10-01T08:30:00-03:00", 100, 120},
		{"2099-10-01T12:00:00Z", 20, 30},
	} {
		_, err := c.g.Publicar(c.m1, protocolo.PublicacaoCarona{Chave: fmt.Sprint(i), Rota: []string{"Feira", "Salvador"}, DataHora: p.hora, Assentos: 1, Trechos: []protocolo.OfertaTrecho{{Preco: p.preco, DistanciaKM: 100, TempoViagem: p.duracao}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	r, err := c.g.Buscar(c.p, protocolo.BuscaItinerario{Origem: "Feira", Destino: "Salvador", Data: "2099-10-01"})
	if err != nil || len(r.Itinerarios) != 3 {
		t.Fatalf("busca: %+v %v", r, err)
	}
	for i, preco := range []float64{100, 20, 1} {
		if r.Itinerarios[i].PrecoTotal != preco {
			t.Fatalf("ordem incorreta: %+v", r)
		}
	}
}
