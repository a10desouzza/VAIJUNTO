// Arquivo: internal/servidor/transacoes.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Controla os assentos da empresa que participam de uma reserva distribuída.
// A preparação separa vagas; a confirmação as ocupa; o aborto as devolve.
// As mudanças são protegidas por travas e repetem-se sem descontar ou devolver vagas duas vezes.

package servidor

import (
	"fmt"
	"strings"
	"time"

	"vaijunto/internal/configuracao"
	"vaijunto/internal/protocolo"
)

// transacaoLocal guarda os assentos separados e o estado da operacao local.
type transacaoLocal struct {
	pedido    protocolo.PedidoPreparacao
	estado    protocolo.EstadoTransacao
	assentos  map[string]int
	motivo    string
	reservaID string
	ordem     uint64
}

// validarPreparacao valida identidade, participantes, horario e lista de trechos do pedido de preparacao.
// Recebe: pedido protocolo.PedidoPreparacao.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
func validarPreparacao(pedido protocolo.PedidoPreparacao) error {
	if strings.TrimSpace(pedido.TransacaoID) == "" || len(pedido.TransacaoID) > 128 {
		return fmt.Errorf("transacao invalida")
	}

	if err := configuracao.ValidarEmpresaID(pedido.EmpresaOrigem); err != nil {
		return err
	}

	if strings.TrimSpace(pedido.PassageiroRef) == "" || len(pedido.PassageiroRef) > 128 {
		return fmt.Errorf("referencia de passageiro invalida")
	}
	if pedido.InicioItinerario != "" {
		if _, err := time.Parse(time.RFC3339, pedido.InicioItinerario); err != nil {
			return fmt.Errorf("início do itinerário inválido")
		}
	}

	if len(pedido.Participantes) == 0 {
		return fmt.Errorf("nenhum participante informado")
	}

	participantes := make(map[string]bool)

	for _, empresa := range pedido.Participantes {
		if err := configuracao.ValidarEmpresaID(empresa); err != nil {
			return err
		}

		if participantes[empresa] {
			return fmt.Errorf("empresa %s informada mais de uma vez", empresa)
		}
		participantes[empresa] = true
	}

	if len(pedido.TrechosIDs) < 1 || len(pedido.TrechosIDs) > 20 {
		return fmt.Errorf("informe de 1 a 20 trechos locais")
	}
	trechos := make(map[string]bool)

	for _, id := range pedido.TrechosIDs {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("trecho invalido")
		}

		if trechos[id] {
			return fmt.Errorf("trecho %s informado mais de uma vez", id)
		}

		trechos[id] = true
	}

	return nil
}

// confirmarTransacao aplica uma decisao recebida do protocolo, sem descontar
// novamente as vagas. Um passageiro externo nunca vira uma conta local.
// Recebe: id string.
// Retorna: a reserva local confirmada ou erro de estado; repetir a mesma confirmacao nao desconta vagas novamente.
func (g *GrafoItinerarios) confirmarTransacao(id string) (protocolo.Reserva, error) {
	g.manutencao.RLock()
	defer g.manutencao.RUnlock()
	g.mu.RLock()
	t := g.transacoes[id]
	if t == nil {
		g.mu.RUnlock()
		return protocolo.Reserva{}, fmt.Errorf("preparação não encontrada")
	}
	ids := append([]string(nil), t.pedido.TrechosIDs...)
	g.mu.RUnlock()
	estados, err := g.localizarTrechos(ids)
	if err != nil {
		return protocolo.Reserva{}, err
	}
	liberar := travarTrechos(estados)
	defer liberar()
	g.mu.Lock()
	defer g.mu.Unlock()
	t = g.transacoes[id]
	if t.estado == protocolo.TransacaoConfirmada {
		return copiarReserva(g.reservas[t.reservaID]), nil
	}
	if t.estado != protocolo.TransacaoPreparada {
		return protocolo.Reserva{}, fmt.Errorf("transação não está preparada")
	}
	for tid, n := range t.assentos {
		if estados[tid].preparados[n] != id {
			return protocolo.Reserva{}, fmt.Errorf("assento preparado inconsistente")
		}
	}
	r := protocolo.Reserva{ID: g.novoID("R"), TransacaoID: id, EmpresaOrigem: t.pedido.EmpresaOrigem, PassageiroRef: t.pedido.PassageiroRef, InicioItinerario: t.pedido.InicioItinerario, Status: protocolo.Ativa, CriadaEm: g.agora().UTC().Format(time.RFC3339), Assentos: make([]protocolo.AssentoReservado, 0, len(ids))}
	var total int64
	for _, tid := range ids {
		e := estados[tid]
		n := t.assentos[tid]
		delete(e.preparados, n)
		e.ocupados[n] = r.ID
		r.Assentos = append(r.Assentos, protocolo.AssentoReservado{Trecho: e.trecho, Numero: n})
		total += centavos(e.trecho.Preco)
	}
	r.PrecoTotal = float64(total) / 100
	g.reservas[r.ID] = r
	t.reservaID = r.ID
	t.estado = protocolo.TransacaoConfirmada
	return copiarReserva(r), nil
}

// cancelarTransacaoConfirmada localiza a transacao confirmada e devolve seus assentos sob exclusao de manutencao; somente o protocolo deve autorizar essa aplicacao.
// Recebe: id, motivo string.
// Retorna: a reserva local cancelada ou erro quando a transacao confirmada nao existe.
func (g *GrafoItinerarios) cancelarTransacaoConfirmada(id, motivo string) (protocolo.Reserva, error) {
	g.manutencao.Lock()
	defer g.manutencao.Unlock()
	g.mu.Lock()
	defer g.mu.Unlock()
	t := g.transacoes[id]
	if t == nil || t.estado != protocolo.TransacaoConfirmada {
		return protocolo.Reserva{}, fmt.Errorf("transação confirmada não encontrada")
	}
	return g.cancelarReserva(t.reservaID, motivo), nil
}

// prepararTransacao escolhe todos os assentos antes de descontar vagas.
// Pedidos repetidos retornam o estado registrado sem separar novas vagas.
// Recebe: pedido protocolo.PedidoPreparacao.
// Retorna: o estado da preparacao local ou erro; so separa vagas depois de validar todos os trechos locais.
func (g *GrafoItinerarios) prepararTransacao(
	pedido protocolo.PedidoPreparacao,
) (protocolo.RespostaPreparacao, error) {
	if err := validarPreparacao(pedido); err != nil {
		return protocolo.RespostaPreparacao{}, err
	}

	g.manutencao.RLock()
	defer g.manutencao.RUnlock()

	estados, err := g.localizarTrechos(pedido.TrechosIDs)
	if err != nil {
		return protocolo.RespostaPreparacao{}, err
	}

	liberar := travarTrechos(estados)
	defer liberar()

	g.mu.Lock()
	defer g.mu.Unlock()

	if anterior, existe := g.transacoes[pedido.TransacaoID]; existe {
		if assinatura(anterior.pedido) != assinatura(pedido) {
			return protocolo.RespostaPreparacao{},
				fmt.Errorf("transação já utilizada com outros dados")
		}

		return protocolo.RespostaPreparacao{
			TransacaoID: pedido.TransacaoID,
			Estado:      anterior.estado,
			Motivo:      anterior.motivo,
			ReservaID:   anterior.reservaID,
		}, nil
	}

	assentos := make(map[string]int)
	agora := g.agora()

	for _, id := range pedido.TrechosIDs {
		estado := estados[id]

		if !g.disponivel(estado.trecho, agora) {
			return protocolo.RespostaPreparacao{},
				fmt.Errorf("trecho %s indisponível", id)
		}

		numero := 0

		for n := 1; n <= estado.trecho.Capacidade; n++ {
			_, ocupado := estado.ocupados[n]
			_, preparado := estado.preparados[n]

			if !ocupado && !preparado {
				numero = n
				break
			}
		}

		if numero == 0 {
			return protocolo.RespostaPreparacao{},
				fmt.Errorf("trecho %s sem assentos livres", id)
		}

		assentos[id] = numero
	}

	pedido.Participantes = append(
		[]string(nil), pedido.Participantes...,
	)

	pedido.TrechosIDs = append(
		[]string(nil), pedido.TrechosIDs...,
	)

	for id, numero := range assentos {
		estado := estados[id]
		estado.preparados[numero] = pedido.TransacaoID
		estado.trecho.Assentos--
	}

	g.transacoes[pedido.TransacaoID] = &transacaoLocal{
		pedido:   pedido,
		estado:   protocolo.TransacaoPreparada,
		assentos: assentos,
		ordem:    g.sequencia + 1,
	}
	g.sequencia++

	return protocolo.RespostaPreparacao{
		TransacaoID: pedido.TransacaoID,
		Estado:      protocolo.TransacaoPreparada,
	}, nil
}

// abortarTransacao libera uma preparacao conhecida apenas uma vez.
// Nao deve ser chamada por timeout sem uma decisao valida de aborto.
// Recebe: id, motivo string.
// Retorna: o estado de aborto ou erro; repeticoes nao devolvem a mesma vaga duas vezes.
func (g *GrafoItinerarios) abortarTransacao(id, motivo string) (protocolo.RespostaPreparacao, error) {
	g.manutencao.RLock()
	defer g.manutencao.RUnlock()

	g.mu.RLock()

	transacao, existe := g.transacoes[id]
	if !existe {
		g.mu.RUnlock()

		return protocolo.RespostaPreparacao{}, fmt.Errorf("transacao %s nao encontrada", id)
	}

	ids := append([]string(nil), transacao.pedido.TrechosIDs...)
	g.mu.RUnlock()

	estados, err := g.localizarTrechos(ids)
	if err != nil {
		return protocolo.RespostaPreparacao{}, err
	}

	liberar := travarTrechos(estados)
	defer liberar()

	g.mu.Lock()
	defer g.mu.Unlock()

	transacao = g.transacoes[id]
	if transacao.estado == protocolo.TransacaoConfirmada {
		return protocolo.RespostaPreparacao{}, fmt.Errorf("transacao %s ja confirmada", id)
	}

	if transacao.estado == protocolo.TransacaoAbortada {
		return protocolo.RespostaPreparacao{TransacaoID: id, Estado: protocolo.TransacaoAbortada, Motivo: transacao.motivo}, nil
	}

	if transacao.estado != protocolo.TransacaoPreparada {
		return protocolo.RespostaPreparacao{}, fmt.Errorf("transacao %s em estado inconsistente", id)
	}

	for trechoID, numero := range transacao.assentos {
		if estados[trechoID].preparados[numero] != id {
			return protocolo.RespostaPreparacao{}, fmt.Errorf("assento preparado inconsistente")
		}
	}

	for trechoID, numero := range transacao.assentos {
		estado := estados[trechoID]

		delete(estado.preparados, numero)
		estado.trecho.Assentos++
	}

	transacao.estado = protocolo.TransacaoAbortada
	transacao.motivo = motivo

	return protocolo.RespostaPreparacao{TransacaoID: id, Estado: protocolo.TransacaoAbortada, Motivo: motivo}, nil
}
