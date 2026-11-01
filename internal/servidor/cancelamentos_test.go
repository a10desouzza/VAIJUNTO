// Arquivo: internal/servidor/cancelamentos_test.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Testa cancelamentos e notificações respeitando o início programado das viagens.
// Verifica devolução das vagas e proteção das conexões de um itinerário já iniciado.
// Também exercita a disputa entre cancelamento e confirmação.

package servidor

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"vaijunto/internal/protocolo"
)

// TestCancelamentoAntesDaPartidaENotificacoes: Cancela antes da partida, repete a operacao e
// confere devolucao unica de vagas, status parcial e propriedade das notificacoes.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
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
		if c.g.trechos[a.Trecho.ID].trecho.Assentos != 2 {
			t.Fatal("vaga não devolvida ou devolvida duas vezes")
		}
	}
	if c.g.caronas[c.bc.ID].status != protocolo.Parcial {
		t.Fatal("carona deveria ser parcial")
	}
	if c.g.trechos[c.bc.Trechos[1].Trecho.ID].trecho.Status != protocolo.Ativa {
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
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
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
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
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
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
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
						trecho := c.g.trechos[a.Trecho.ID].trecho
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
					if c.g.trechos[a.Trecho.ID].trecho.Assentos != 2 || len(c.g.trechos[a.Trecho.ID].ocupados) != 0 {
						t.Fatal("vagas nao devolvidas exatamente uma vez")
					}
				}
				if c.g.caronas[c.ab.ID].status != protocolo.Ativa || c.g.trechos[c.ab.Trechos[0].Trecho.ID].trecho.Status != protocolo.Ativa {
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

// TestCancelamentoEConfirmacaoConcorrentes: Dispara confirmacoes e cancelamentos simultaneos. Ao
// final confere vagas devolvidas e ausencia de reserva ativa no trecho cancelado.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
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
		if c.g.trechos[id].trecho.Assentos != 2 || len(c.g.trechos[id].ocupados) != 0 {
			t.Fatal("vagas inconsistentes após disputa")
		}
	}
	for _, r := range c.g.reservas {
		if r.Status != protocolo.Cancelada {
			t.Fatal("reserva ativa em trecho cancelado")
		}
	}
}

// Cancelar varios trechos da mesma reserva devolve cada vaga e notifica apenas uma vez.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestCancelarCaronaComReservaEmVariosTrechos(t *testing.T) {
	c := criarCenario(t)
	ids := []string{c.bc.Trechos[0].Trecho.ID, c.bc.Trechos[1].Trecho.ID}
	r, err := c.g.Confirmar(c.p, protocolo.ReservaItinerario{Chave: "bcd", TrechosIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := c.g.CancelarCarona(c.m2, c.bc.ID); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range ids {
		if c.g.trechos[id].trecho.Assentos != 2 || len(c.g.trechos[id].ocupados) != 0 {
			t.Fatal("vagas devolvidas incorretamente")
		}
	}
	avisos, err := c.g.ConsultarNotificacoes(c.p)
	if err != nil || len(avisos) != 1 || avisos[0].ReservaID != r.ID || c.g.reservas[r.ID].Status != protocolo.Cancelada {
		t.Fatalf("cancelamento ou aviso duplicado: %+v %v", avisos, err)
	}
}
