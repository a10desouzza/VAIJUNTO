// Arquivo: internal/servidor/transacoes_test.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Verifica preparação e aborto dos assentos locais de transações.
// Testa recusa sem preparação parcial, idempotência e proteção das vagas sob concorrência.
// Também verifica a independência de operações em trechos diferentes.

package servidor

import (
	"strings"
	"testing"
	"time"

	"vaijunto/internal/protocolo"
)

// TestPreparacaoEAbortoPreservamVagas verifica o cenario: Preparacao EAborto Preservam Vagas.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
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

// TestPreparacaoRecusadaNaoSeparaParteDoPedido verifica o cenario: Preparacao Recusada Nao Separa Parte Do Pedido.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
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

// TestValidarPreparacaoRejeitaLimitesInvalidos verifica o cenario: Validar Preparacao Rejeita Limites Invalidos.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestValidarPreparacaoRejeitaLimitesInvalidos(t *testing.T) {
	base := protocolo.PedidoPreparacao{TransacaoID: "teste", EmpresaOrigem: "empresa-a", PassageiroRef: "passageiro-teste", Participantes: []string{"empresa-b"}, TrechosIDs: []string{"T1"}}
	casos := []protocolo.PedidoPreparacao{base, base, base, base, base, base, base}
	casos[0].TransacaoID = strings.Repeat("x", 129)
	casos[1].TrechosIDs = nil
	casos[2].TrechosIDs = make([]string, 21)
	casos[3].TrechosIDs = []string{"T1", "T1"}
	casos[4].PassageiroRef = ""
	casos[5].PassageiroRef = "   "
	casos[6].PassageiroRef = strings.Repeat("p", 129)
	for i, p := range casos {
		if validarPreparacao(p) == nil {
			t.Fatalf("caso invalido aceito: %d", i)
		}
	}
	if err := validarPreparacao(base); err != nil {
		t.Fatal(err)
	}
}

// TestChaveConcorrenteEmTrechosDiferentesNaoOcupaDuasVagas verifica o cenario: Chave Concorrente Em Trechos Diferentes Nao Ocupa Duas Vagas.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
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

// TestReservaEmOutroTrechoNaoEsperaTravaDoPrimeiro verifica o cenario: Reserva Em Outro Trecho Nao Espera Trava Do Primeiro.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
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
