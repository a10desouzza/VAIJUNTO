// testes/concorrencia_test.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Testes de autenticacao, grafo e integridade das reservas.
// Cada cenario cria seu proprio estado para evitar dependencia entre testes.

package testes

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"vaijunto/internal/protocolo"
)

// TestConcorrenciaUltimoAssento: Disputa a ultima vaga com goroutines e confere que apenas uma
// reserva completa foi confirmada.
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
			sessaoID := a.passageiro
			if i%2 == 1 {
				sessaoID = a.outroPassageiro
			}
			if _, err := a.g.Confirmar(sessaoID, protocolo.ReservaItinerario{Chave: fmt.Sprint(i), TrechosIDs: ids(c)}); err == nil {
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

// TestAtomicidadeEAssentoPorTrecho: Confere que uma falha em um trecho nao ocupa os demais e que a
// disponibilidade e controlada por trecho.
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

// TestIdempotenciaECancelamento: Repete publicacoes, confirmacoes e cancelamentos para verificar
// que nao duplicam recursos nem vagas.
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
		cancelada, err := a.g.CancelarReserva(a.passageiro, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		// A repeticao deve devolver a mesma reserva com todos os dados do cancelamento.
		if cancelada.ID != r.ID || cancelada.Status != protocolo.Cancelada || cancelada.Motivo == "" || cancelada.CanceladaEm == "" {
			t.Fatalf("cancelamento incompleto ou inconsistente: %+v", cancelada)
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
