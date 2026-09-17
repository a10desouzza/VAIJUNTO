// internal/servidor/cancelamentos_test.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Testes de cancelamento antes e depois do inicio da viagem e notificacoes.

package servidor

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"vaijunto/internal/protocolo"
)

// TestCancelamentoAntesDaPartidaENotificacoes: Cancela antes da partida, repete a operacao e
// confere devolucao unica de vagas, status parcial e propriedade das notificacoes.
func TestCancelamentoAntesDaPartidaENotificacoes(t *testing.T) {
	c := criarCenario(t)
	r := c.reservar(t)
	c.agora = c.agora.Add(29*time.Minute + 59*time.Second)
	c.reautenticar(t)
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

// TestCaronaIniciadaPermiteReservaDeTrechoFuturo: Simula carona iniciada, recusa cancelamento e
// permite nova reserva em um trecho posterior que ainda nao partiu.
func TestCaronaIniciadaPermiteReservaDeTrechoFuturo(t *testing.T) {
	c := criarCenario(t)
	c.agora = c.agora.Add(50 * time.Minute)
	c.reautenticar(t)
	if _, err := c.g.CancelarCarona(c.m2, c.bc.ID); err == nil {
		t.Fatal("carona iniciada cancelada")
	}
	tid := c.bc.Trechos[1].Trecho.ID
	if _, err := c.g.CancelarTrecho(c.m2, tid); err == nil {
		t.Fatal("trecho futuro de carona iniciada cancelado")
	}
	if _, err := c.g.Confirmar(c.p, protocolo.ReservaItinerario{Chave: "tarde", TrechosIDs: []string{tid}}); err != nil {
		t.Fatal(err)
	}
	resultado, err := c.g.Buscar(c.p, protocolo.BuscaItinerario{Origem: "C", Destino: "D", Data: "2099-10-01"})
	if err != nil || len(resultado.Itinerarios) != 1 {
		t.Fatal("busca deveria retornar trecho futuro", err)
	}
}

// TestItinerarioIniciadoNaoBloqueiaTrechoSemRelacao: Cancela um trecho que nao pertence ao
// itinerario iniciado e verifica que a reserva independente permanece ativa.
func TestItinerarioIniciadoNaoBloqueiaTrechoSemRelacao(t *testing.T) {
	c := criarCenario(t)
	r := c.reservar(t)
	c.agora = c.agora.Add(31 * time.Minute)
	c.reautenticar(t)
	if _, err := c.g.CancelarTrecho(c.m2, c.bc.Trechos[1].Trecho.ID); err != nil {
		t.Fatal(err)
	}
	if c.g.reservas[r.ID].Status != protocolo.Ativa {
		t.Fatal("reserva independente cancelada")
	}
}

// O cancelamento do segundo motorista afeta a reserva inteira, mas nao
// cancela a oferta independente do primeiro motorista.
func TestCancelamentoMotoristaDoisRespeitaInicioDoItinerario(t *testing.T) {
	for _, tipo := range []string{"trecho", "carona"} {
		for _, deslocamento := range []time.Duration{-time.Second, 0, time.Second} {
			t.Run(fmt.Sprintf("%s/%s", tipo, deslocamento), func(t *testing.T) {
				c := criarCenario(t)
				r := c.reservar(t)
				c.agora = partida(r.Assentos[0].Trecho).Add(deslocamento)
				c.reautenticar(t)
				if tipo == "carona" {
					if _, err := c.g.CancelarCarona(c.m1, c.bc.ID); err == nil {
						t.Fatal("outro motorista cancelou a carona")
					}
				}
				err := c.cancelarConexao(tipo)
				if deslocamento >= 0 {
					if err == nil || !strings.Contains(err.Error(), "já iniciou o itinerário") {
						t.Fatalf("deveria proteger conexão: %v", err)
					}
					if c.g.reservas[r.ID].Status != protocolo.Ativa || c.g.caronas[c.bc.ID].status != protocolo.Ativa || len(c.g.notificacoes[r.Passageiro]) != 0 {
						t.Fatal("cancelamento recusado alterou reserva ou avisos")
					}
					for _, a := range r.Assentos {
						trecho := c.g.trechos[a.Trecho.ID]
						if trecho.Status != protocolo.Ativa || trecho.Assentos != 1 {
							t.Fatal("cancelamento recusado alterou trecho ou vagas")
						}
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if c.g.reservas[r.ID].Status != protocolo.Cancelada {
					t.Fatal("nao cancelou A-B-C por inteiro")
				}
				if err := c.cancelarConexao(tipo); err != nil {
					t.Fatal(err)
				}
				for _, a := range r.Assentos {
					if c.g.trechos[a.Trecho.ID].Assentos != 2 || len(c.g.ocupados[a.Trecho.ID]) != 0 {
						t.Fatal("vagas nao devolvidas exatamente uma vez")
					}
				}
				if c.g.caronas[c.ab.ID].status != protocolo.Ativa || c.g.trechos[c.ab.Trechos[0].Trecho.ID].Status != protocolo.Ativa {
					t.Fatal("oferta do motorista 1 foi cancelada indevidamente")
				}
				busca, err := c.g.Buscar(c.p, protocolo.BuscaItinerario{Origem: "B", Destino: "C", Data: "2099-10-01"})
				if err != nil || len(busca.Itinerarios) != 0 {
					t.Fatal("trecho cancelado apareceu na busca", err)
				}
				avisos, err := c.g.ConsultarNotificacoes(c.p)
				if err != nil || len(avisos) != 1 || avisos[0].ReservaID != r.ID {
					t.Fatalf("aviso de cancelamento: %+v %v", avisos, err)
				}
			})
		}
	}
}
