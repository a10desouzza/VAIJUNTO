// TEC502 - Problema 2. Autor: Arthur de Andrade Souza.
// Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
package servidor

import (
	"fmt"
	"math"
	"time"
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

// confirmarTransacao aplica uma decisao recebida do protocolo, sem descontar
// novamente as vagas. Um passageiro externo nunca vira uma conta local.
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

// decimalValido rejeita negativos, NaN, infinitos, valores acima do limite ou com mais de duas
// casas decimais.
func decimalValido(valor, limite float64) bool {
	return !math.IsNaN(valor) && !math.IsInf(valor, 0) && valor >= 0 && valor <= limite && math.Abs(valor*100-math.Round(valor*100)) < 0.000001
}

// iniciou compara a partida da carona com o relogio. No horario exato da partida ela ja iniciou.
func (g *GrafoItinerarios) iniciou(o *oferta, agora time.Time) bool {
	h, err := time.Parse(time.RFC3339, o.entrada.DataHora)
	return err != nil || !h.After(agora)
}

// disponivel exige trecho ativo, vagas e partida do trecho no futuro. Quem chama protege os mapas
// com trava.
func (g *GrafoItinerarios) disponivel(t protocolo.Trecho, agora time.Time) bool {
	o, ok := g.caronas[t.CaronaID]
	return ok && o.status != protocolo.Cancelada && t.Status == protocolo.Ativa && t.Assentos > 0 && partida(t).After(agora)
}

// validarCancelamento impede cancelar conexoes de reservas ativas cujo primeiro trecho ja
// iniciou. Examina o itinerario inteiro, inclusive trechos de outros motoristas. Exige trava.
func (g *GrafoItinerarios) validarCancelamento(ids []string, agora time.Time) error {
	selecionados := make(map[string]bool, len(ids))
	for _, id := range ids {
		// Uma preparacao pendente ainda depende de uma decisao conjunta.
		// Cancelar a oferta aqui invalidaria as vagas prometidas ao protocolo.
		if len(g.trechos[id].preparados) > 0 {
			return fmt.Errorf("cancelamento recusado: trecho %s tem transação preparada", id)
		}
		for _, rid := range g.trechos[id].ocupados {
			if g.reservas[rid].TransacaoID != "" {
				return fmt.Errorf("cancele a reserva distribuída pelo protocolo antes de alterar a oferta")
			}
		}
		selecionados[id] = true
	}
	/* Basta o PRIMEIRO trecho da reserva ter iniciado para proteger suas conexoes futuras. */
	for _, r := range g.reservas {
		inicio := partida(r.Assentos[0].Trecho)
		if r.InicioItinerario != "" {
			inicio, _ = time.Parse(time.RFC3339, r.InicioItinerario)
		}
		if r.Status != protocolo.Ativa || inicio.After(agora) {
			continue
		}
		for _, a := range r.Assentos {
			if selecionados[a.Trecho.ID] {
				return fmt.Errorf("cancelamento recusado: um passageiro já iniciou o itinerário da reserva %s", r.ID)
			}
		}
	}
	return nil
}
