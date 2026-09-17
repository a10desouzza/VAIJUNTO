// internal/servidor/cancelamentos_integridade_test.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Integridade das vagas na disputa e na repeticao dos cancelamentos.
// O roteamento de cancelamentos e avisos e exercitado no teste completo dos menus via TCP.

package servidor

import (
	"fmt"
	"sync"
	"testing"
	"vaijunto/internal/protocolo"
)

// TestCancelamentoEConfirmacaoConcorrentes: Dispara confirmacoes e cancelamentos simultaneos. Ao
// final confere vagas devolvidas e ausencia de reserva ativa no trecho cancelado.
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

// Cancelar varios trechos da mesma reserva devolve cada vaga e notifica apenas uma vez.
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
		if c.g.trechos[id].Assentos != 2 || len(c.g.ocupados[id]) != 0 {
			t.Fatal("vagas devolvidas incorretamente")
		}
	}
	avisos, err := c.g.ConsultarNotificacoes(c.p)
	if err != nil || len(avisos) != 1 || avisos[0].ReservaID != r.ID || c.g.reservas[r.ID].Status != protocolo.Cancelada {
		t.Fatalf("cancelamento ou aviso duplicado: %+v %v", avisos, err)
	}
}
