package servidor

import (
	"fmt"
	"math"
	"time"
	"vaijunto/internal/protocolo"
)

func decimalValido(valor, limite float64) bool {
	return !math.IsNaN(valor) && !math.IsInf(valor, 0) && valor >= 0 && valor <= limite && math.Abs(valor*100-math.Round(valor*100)) < 0.000001
}

func (g *GrafoItinerarios) iniciou(o *oferta, agora time.Time) bool {
	h, err := time.Parse(time.RFC3339, o.entrada.DataHora)
	return err != nil || !h.After(agora)
}

func (g *GrafoItinerarios) disponivel(t protocolo.Trecho, agora time.Time) bool {
	o, ok := g.caronas[t.CaronaID]
	return ok && o.status != protocolo.Cancelada && t.Status == protocolo.Ativa && t.Assentos > 0 && !g.iniciou(o, agora)
}

func (g *GrafoItinerarios) validarCancelamento(ids []string, agora time.Time) error {
	selecionados := make(map[string]bool, len(ids))
	for _, id := range ids {
		selecionados[id] = true
	}
	for _, r := range g.reservas {
		if r.Status != protocolo.Ativa || partida(r.Assentos[0].Trecho).After(agora) {
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

func (g *GrafoItinerarios) ConsultarNotificacoes(token string) ([]protocolo.Notificacao, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	u, err := g.autorizar(token, "")
	if err != nil {
		return nil, err
	}
	return append([]protocolo.Notificacao{}, g.notificacoes[u.Email]...), nil
}

func (g *GrafoItinerarios) LerNotificacao(token, id string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	u, err := g.autorizar(token, "")
	if err != nil {
		return err
	}
	for i, n := range g.notificacoes[u.Email] {
		if n.ID == id {
			g.notificacoes[u.Email][i].Lida = true
			return nil
		}
	}
	return fmt.Errorf("notificação não encontrada para este usuário")
}

func (g *GrafoItinerarios) notificar(r protocolo.Reserva) {
	n := protocolo.Notificacao{ID: g.novoID("N"), ReservaID: r.ID, Mensagem: r.Motivo, CriadaEm: g.agora().UTC().Format(time.RFC3339)}
	g.notificacoes[r.Passageiro] = append(g.notificacoes[r.Passageiro], n)
}

func (g *GrafoItinerarios) CancelarTrecho(token, id string) (protocolo.Carona, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	u, err := g.autorizar(token, protocolo.Motorista)
	if err != nil {
		return protocolo.Carona{}, err
	}
	t, ok := g.trechos[id]
	if !ok || t.Motorista != u.Email {
		return protocolo.Carona{}, fmt.Errorf("trecho não encontrado para este motorista")
	}
	o := g.caronas[t.CaronaID]
	if t.Status == protocolo.Cancelada {
		return g.consultarCarona(t.CaronaID), nil
	}
	agora := g.agora()
	if g.iniciou(o, agora) {
		return protocolo.Carona{}, fmt.Errorf("não é possível cancelar trecho de carona já iniciada")
	}
	if err := g.validarCancelamento([]string{id}, agora); err != nil {
		return protocolo.Carona{}, err
	}
	t.Status = protocolo.Cancelada
	g.trechos[id] = t
	for rid, r := range g.reservas {
		if r.Status != protocolo.Ativa {
			continue
		}
		for _, a := range r.Assentos {
			if a.Trecho.ID == id {
				cancelada := g.cancelarReserva(rid, "Trecho "+t.Origem+" -> "+t.Destino+" cancelado pelo motorista; itinerário inteiro cancelado.")
				g.notificar(cancelada)
				break
			}
		}
	}
	o.status = protocolo.Cancelada
	for _, tid := range o.ids {
		if g.trechos[tid].Status == protocolo.Ativa {
			o.status = protocolo.Parcial
			break
		}
	}
	return g.consultarCarona(t.CaronaID), nil
}
