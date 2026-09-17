// testes/falhas_test.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Recuperacao de resposta perdida e cancelamentos concorrentes.
// Repetir nao deve criar outra reserva nem devolver a mesma vaga duas vezes.

package testes

import (
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"
	"vaijunto/internal/protocolo"
	"vaijunto/internal/servidor"
)

// TestRespostaPerdidaPodeSerRecuperada: Fecha a conexao apos enviar a confirmacao, repete a mesma
// chave e verifica que existe somente uma reserva.
func TestRespostaPerdidaPodeSerRecuperada(t *testing.T) {
	a := preparar(t)
	c := publicar(t, a, a.motorista, "ab", []string{"A", "B"}, "2099-10-01T08:00:00-03:00", 1)
	pedido := protocolo.ReservaItinerario{Chave: "resposta-perdida", TrechosIDs: ids(c)}
	dados, _ := json.Marshal(pedido)
	mensagem, _ := json.Marshal(protocolo.Requisicao{Acao: protocolo.AcaoConfirmar, SessaoID: a.passageiro, Dados: dados})
	servidorConn, clienteConn := net.Pipe()
	fim := make(chan struct{})
	go func() { defer close(fim); servidor.AtenderConexao(servidorConn, a.g, time.Second) }()
	clienteConn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := clienteConn.Write(append(mensagem, '\n')); err != nil {
		t.Fatal(err)
	}
	clienteConn.Close()
	select {
	case <-fim:
	case <-time.After(2 * time.Second):
		t.Fatal("servidor preso no cliente desconectado")
	}
	r, err := a.g.Confirmar(a.passageiro, pedido)
	if err != nil {
		t.Fatal(err)
	}
	reservas, _ := a.g.ConsultarReservas(a.passageiro)
	if len(reservas) != 1 || reservas[0].ID != r.ID {
		t.Fatal("resposta perdida duplicou reserva")
	}
}

// TestCancelamentoConcorrente: Alterna cancelamentos de reserva e carona em goroutines e confere
// que cada vaga foi devolvida uma unica vez.
func TestCancelamentoConcorrente(t *testing.T) {
	a := preparar(t)
	c := publicar(t, a, a.motorista, "abc", []string{"A", "B", "C"}, "2099-10-01T08:00:00-03:00", 1)
	r, err := a.g.Confirmar(a.passageiro, protocolo.ReservaItinerario{Chave: "r", TrechosIDs: ids(c)})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if i%2 == 0 {
				a.g.CancelarReserva(a.passageiro, r.ID)
			} else {
				a.g.CancelarCarona(a.motorista, c.ID)
			}
		}()
	}
	wg.Wait()
	caronas, _ := a.g.ConsultarCaronas(a.motorista)
	for _, trecho := range caronas[0].Trechos {
		if trecho.Trecho.Assentos != 1 || len(trecho.Passageiros) != 0 {
			t.Fatal("liberação duplicada de assento")
		}
	}
}
