// Autor: Arthur de Andrade Souza.
// Matéria: TEC502 - TP01 - Concorrência e Conectividade - 2026.2.
// Declaração de não plágio: Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
// Funcionalidade: Testa preparação e aborto de assentos, conexões temporais e 2PC com relógios físicos distintos.
package servidor

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
	"vaijunto/internal/protocolo"
)

// TestLamport2PCComRelogiosFisicosDiferentes verifica propagação causal REST e reserva 2PC com relógios físicos distintos.
// Recebe: controle de teste t.
func TestLamport2PCComRelogiosFisicosDiferentes(t *testing.T) {
	es := criarConsorcio(t, 1)
	agora := time.Now()
	for i, e := range es {
		desvio := time.Duration(i*48-24) * time.Hour
		e.g.agora = func() time.Time { return agora.Add(desvio) }
	}
	a, b := es[0].d, es[1].d
	a.evento(2000)
	b.evento(1000)
	var saude saudeEmpresa
	if err := a.chamar(context.Background(), b.id, "saude", nil, &saude); err != nil {
		t.Fatal(err)
	}
	if a.relogio <= b.relogio || b.relogio <= 2000 {
		t.Fatal("ida e volta REST nao propagaram a causalidade")
	}
	r, err := a.reservar(es[0].passageiro, pedidoTeste(es, "lamport"))
	if err != nil || r.Status != protocolo.Ativa {
		t.Fatalf("2PC com horarios diferentes: %+v %v", r, err)
	}
	marca := a.registros[r.TransacaoID].Meta.Lamport
	for _, e := range es {
		if marca == 0 || e.g.transacoes[r.TransacaoID].ordem <= marca {
			t.Fatal("preparacao nao sucedeu a criacao logica da transacao")
		}
	}
	verificarVagas(t, es, 0, 1, 0)
}

// TestPublicacaoSemParadaNoDestinoFinal verifica a regra de parada apenas nas conexões, não após o destino final.
// Recebe: controle de teste t.
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

// TestReservaSerrinhaFeiraSalvadorDuranteViagemEParada verifica horários e cancelamentos durante a viagem e a parada entre conexões.
// Recebe: controle de teste t.
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
			if c.g.trechos[segundo].trecho.Assentos != 1 {
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

// TestPreparacaoEAbortoPreservamVagas verifica separação e liberação dos assentos sem duplicar os efeitos.
// Recebe: controle de teste t.
func TestPreparacaoEAbortoPreservamVagas(t *testing.T) {
	c := criarCenario(t)
	id := c.ab.Trechos[0].Trecho.ID
	pedido := protocolo.PedidoPreparacao{TransacaoID: "empresa-a:1", EmpresaOrigem: "empresa-a", PassageiroRef: "passageiro-teste", Participantes: []string{"empresa-b"}, TrechosIDs: []string{id}}
	for i := 0; i < 2; i++ {
		r, err := c.g.prepararTransacao(pedido)
		if err != nil || r.Estado != protocolo.TransacaoPreparada {
			t.Fatalf("preparar: %+v %v", r, err)
		}
	}
	if c.g.trechos[id].trecho.Assentos != 1 || len(c.g.trechos[id].preparados) != 1 {
		t.Fatal("preparacao repetida descontou mais de uma vaga")
	}
	if _, err := c.g.CancelarCarona(c.m1, c.ab.ID); err == nil {
		t.Fatal("carona com preparacao pendente foi cancelada")
	}
	if _, err := c.g.CancelarTrecho(c.m1, id); err == nil {
		t.Fatal("trecho com preparacao pendente foi cancelado")
	}
	r, err := c.g.Confirmar(c.p, protocolo.ReservaItinerario{Chave: "local", TrechosIDs: []string{id}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Assentos[0].Numero == c.g.transacoes[pedido.TransacaoID].assentos[id] {
		t.Fatal("reserva local usou assento preparado")
	}
	for i := 0; i < 2; i++ {
		r, err := c.g.abortarTransacao(pedido.TransacaoID, "outro participante recusou")
		if err != nil || r.Estado != protocolo.TransacaoAbortada {
			t.Fatalf("abortar: %+v %v", r, err)
		}
	}
	if c.g.trechos[id].trecho.Assentos != 1 || len(c.g.trechos[id].preparados) != 0 || len(c.g.trechos[id].ocupados) != 1 {
		t.Fatal("aborto devolveu vagas incorretamente")
	}
	repeticao, err := c.g.prepararTransacao(pedido)
	if err != nil || repeticao.Estado != protocolo.TransacaoAbortada {
		t.Fatal("preparacao atrasada reabriu transacao abortada")
	}
	if _, err := c.g.CancelarReserva(c.p, r.ID); err != nil {
		t.Fatal(err)
	}
	if c.g.trechos[id].trecho.Assentos != 2 {
		t.Fatal("cancelamento local nao devolveu vaga")
	}
}

// TestPreparacaoRecusadaNaoSeparaParteDoPedido verifica que uma preparação recusada não deixa uma alocação local parcial.
// Recebe: controle de teste t.
func TestPreparacaoRecusadaNaoSeparaParteDoPedido(t *testing.T) {
	c := criarCenario(t)
	ab, bc := c.ab.Trechos[0].Trecho.ID, c.bc.Trechos[0].Trecho.ID
	if _, err := c.g.CancelarTrecho(c.m2, bc); err != nil {
		t.Fatal(err)
	}
	p := protocolo.PedidoPreparacao{TransacaoID: "teste", EmpresaOrigem: "empresa-a", PassageiroRef: "passageiro-teste", Participantes: []string{"empresa-b"}, TrechosIDs: []string{ab, bc}}
	if _, err := c.g.prepararTransacao(p); err == nil || !strings.Contains(err.Error(), "indisponível") {
		t.Fatalf("esperava recusa por trecho cancelado; recebeu: %v", err)
	}
	if c.g.trechos[ab].trecho.Assentos != 2 || len(c.g.trechos[ab].preparados) != 0 || len(c.g.transacoes) != 0 {
		t.Fatal("falha deixou preparacao parcial")
	}
}

// TestChaveConcorrenteEmTrechosDiferentesNaoOcupaDuasVagas verifica que a mesma chave concorrente não cria duas reservas distintas.
// Recebe: controle de teste t.
func TestChaveConcorrenteEmTrechosDiferentesNaoOcupaDuasVagas(t *testing.T) {
	c := criarCenario(t)
	ids := []string{c.ab.Trechos[0].Trecho.ID, c.bc.Trechos[0].Trecho.ID}
	inicio := make(chan struct{})
	resultados := make(chan error, 2)
	for _, id := range ids {
		go func(id string) {
			<-inicio
			_, err := c.g.Confirmar(c.p, protocolo.ReservaItinerario{Chave: "unica", TrechosIDs: []string{id}})
			resultados <- err
		}(id)
	}
	close(inicio)
	sucessos := 0
	for range ids {
		select {
		case err := <-resultados:
			if err == nil {
				sucessos++
			} else if !strings.Contains(err.Error(), "chave já utilizada") {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("confirmacoes nao terminaram")
		}
	}
	if sucessos != 1 {
		t.Fatalf("confirmacoes: %d", sucessos)
	}
	c.g.mu.RLock()
	defer c.g.mu.RUnlock()
	if len(c.g.reservas) != 1 || len(c.g.trechos[ids[0]].ocupados)+len(c.g.trechos[ids[1]].ocupados) != 1 {
		t.Fatal("mesma chave ocupou mais de uma vaga")
	}
}

// TestReservaEmOutroTrechoNaoEsperaTravaDoPrimeiro verifica que um trecho independente não precisa aguardar a trava de outro.
// Recebe: controle de teste t.
func TestReservaEmOutroTrechoNaoEsperaTravaDoPrimeiro(t *testing.T) {
	c := criarCenario(t)
	ab := c.ab.Trechos[0].Trecho.ID
	bc := c.bc.Trechos[0].Trecho.ID
	estado := c.g.trechos[ab]
	estado.mu.Lock()
	defer estado.mu.Unlock()
	iniciou := make(chan struct{})
	primeiro := make(chan error, 1)
	go func() {
		close(iniciou)
		_, err := c.g.Confirmar(c.p, protocolo.ReservaItinerario{Chave: "esperando", TrechosIDs: []string{ab}})
		primeiro <- err
	}()
	<-iniciou
	segundo := make(chan error, 1)
	go func() {
		_, err := c.g.Confirmar(c.outro, protocolo.ReservaItinerario{Chave: "independente", TrechosIDs: []string{bc}})
		segundo <- err
	}()
	select {
	case err := <-segundo:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reserva independente esperou pela trava de outro trecho")
	}
	estado.mu.Unlock()
	select {
	case err := <-primeiro:
		estado.mu.Lock()
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		estado.mu.Lock()
		t.Fatal("reserva nao prosseguiu apos liberar seu trecho")
	}
}
