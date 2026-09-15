package servidor

import (
	"fmt"
	"sort"
	"time"
	"vaijunto/internal/protocolo"
)

func copiarReserva(r protocolo.Reserva) protocolo.Reserva {
	r.Assentos = append([]protocolo.AssentoReservado(nil), r.Assentos...)
	return r
}

func (g *GrafoItinerarios) Confirmar(token string, p protocolo.ReservaItinerario) (protocolo.Reserva, error) {
	if err := validarChave(p.Chave); err != nil {
		return protocolo.Reserva{}, err
	}
	if len(p.TrechosIDs) < 1 || len(p.TrechosIDs) > 20 {
		return protocolo.Reserva{}, fmt.Errorf("informe de 1 a 20 trechos em ordem")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	u, err := g.autorizar(token, protocolo.Passageiro)
	if err != nil {
		return protocolo.Reserva{}, err
	}
	chave := u.Email + "|reservar|" + p.Chave
	sig := assinatura(p.TrechosIDs)
	if anterior, ok := g.chaves[chave]; ok {
		if anterior.assinatura != sig {
			return protocolo.Reserva{}, fmt.Errorf("chave já utilizada com outros dados")
		}
		return copiarReserva(g.reservas[anterior.id]), nil
	}
	usados := make(map[string]bool)
	cidades := make(map[string]bool)
	assentos := make([]protocolo.AssentoReservado, 0, len(p.TrechosIDs))
	var anterior protocolo.Trecho
	var total int64
	agora := g.agora()
	for _, id := range p.TrechosIDs {
		t, ok := g.trechos[id]
		if !ok || usados[id] || !g.disponivel(t, agora) {
			return protocolo.Reserva{}, fmt.Errorf("trecho inexistente, repetido, cancelado, sem vagas ou carona já iniciada")
		}
		usados[id] = true
		if len(assentos) == 0 {
			cidades[chaveCidade(t.Origem)] = true
		} else if !conectam(anterior, t) {
			return protocolo.Reserva{}, fmt.Errorf("trechos sem conexão espacial ou temporal")
		}
		if cidades[chaveCidade(t.Destino)] {
			return protocolo.Reserva{}, fmt.Errorf("itinerário contém ciclo")
		}
		cidades[chaveCidade(t.Destino)] = true
		numero := 0
		for n := 1; n <= t.Capacidade; n++ {
			if _, ocupado := g.ocupados[id][n]; !ocupado {
				numero = n
				break
			}
		}
		if numero == 0 {
			return protocolo.Reserva{}, fmt.Errorf("assentos indisponíveis")
		}
		assentos = append(assentos, protocolo.AssentoReservado{Trecho: t, Numero: numero})
		total += centavos(t.Preco)
		anterior = t
	}
	r := protocolo.Reserva{ID: g.novoID("R"), Passageiro: u.Email, Status: protocolo.Ativa, CriadaEm: g.agora().UTC().Format(time.RFC3339), Assentos: assentos, PrecoTotal: float64(total) / 100}
	for i, a := range r.Assentos {
		t := g.trechos[a.Trecho.ID]
		t.Assentos--
		g.trechos[t.ID] = t
		g.ocupados[t.ID][a.Numero] = r.ID
		r.Assentos[i].Trecho = t
	}
	g.reservas[r.ID] = r
	g.chaves[chave] = repeticao{assinatura: sig, id: r.ID}
	return copiarReserva(r), nil
}

func (g *GrafoItinerarios) ConsultarReservas(token string) ([]protocolo.Reserva, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	u, err := g.autorizar(token, protocolo.Passageiro)
	if err != nil {
		return nil, err
	}
	resultado := make([]protocolo.Reserva, 0)
	for _, r := range g.reservas {
		if r.Passageiro == u.Email {
			resultado = append(resultado, copiarReserva(r))
		}
	}
	sort.Slice(resultado, func(i, j int) bool { return resultado[i].ID < resultado[j].ID })
	return resultado, nil
}

func (g *GrafoItinerarios) cancelarReserva(id, motivo string) protocolo.Reserva {
	r := g.reservas[id]
	if r.Status == protocolo.Cancelada {
		return copiarReserva(r)
	}
	for _, a := range r.Assentos {
		t := g.trechos[a.Trecho.ID]
		delete(g.ocupados[t.ID], a.Numero)
		t.Assentos++
		g.trechos[t.ID] = t
	}
	r.Status = protocolo.Cancelada
	r.Motivo = motivo
	r.CanceladaEm = g.agora().UTC().Format(time.RFC3339)
	g.reservas[id] = r
	return copiarReserva(r)
}

func (g *GrafoItinerarios) CancelarReserva(token, id string) (protocolo.Reserva, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	u, err := g.autorizar(token, protocolo.Passageiro)
	if err != nil {
		return protocolo.Reserva{}, err
	}
	r, ok := g.reservas[id]
	if !ok || r.Passageiro != u.Email {
		return protocolo.Reserva{}, fmt.Errorf("reserva não encontrada para este passageiro")
	}
	if r.Status != protocolo.Cancelada && !partida(r.Assentos[0].Trecho).After(g.agora()) {
		return protocolo.Reserva{}, fmt.Errorf("não é possível cancelar uma reserva já iniciada")
	}
	return g.cancelarReserva(id, "Cancelada pelo passageiro."), nil
}

func (g *GrafoItinerarios) CancelarCarona(token, id string) (protocolo.Carona, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	u, err := g.autorizar(token, protocolo.Motorista)
	if err != nil {
		return protocolo.Carona{}, err
	}
	o, ok := g.caronas[id]
	if !ok || o.motorista != u.Email {
		return protocolo.Carona{}, fmt.Errorf("carona não encontrada para este motorista")
	}
	if o.status == protocolo.Cancelada {
		return g.consultarCarona(id), nil
	}
	agora := g.agora()
	if g.iniciou(o, agora) {
		return protocolo.Carona{}, fmt.Errorf("não é possível cancelar uma carona já iniciada")
	}
	if err := g.validarCancelamento(o.ids, agora); err != nil {
		return protocolo.Carona{}, err
	}
	o.status = protocolo.Cancelada
	for _, tid := range o.ids {
		t := g.trechos[tid]
		t.Status = protocolo.Cancelada
		g.trechos[tid] = t
	}
	for rid, r := range g.reservas {
		if r.Status != protocolo.Ativa {
			continue
		}
		for _, a := range r.Assentos {
			if a.Trecho.CaronaID == id {
				cancelada := g.cancelarReserva(rid, "Carona "+id+" cancelada pelo motorista; itinerário inteiro cancelado.")
				g.notificar(cancelada)
				break
			}
		}
	}
	return g.consultarCarona(id), nil
}
