// Autor: Arthur de Andrade Souza.
// Matéria: TEC502 - TP01 - Concorrência e Conectividade - 2026.2.
// Declaração de não plágio: Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
// Funcionalidade: Confirma reservas locais e cancela reservas ou ofertas com validação e devolução dos assentos.
package servidor

import (
	"fmt"
	"sort"
	"time"
	"vaijunto/internal/protocolo"
)

// copiarReserva copia a lista de assentos para a resposta nao compartilhar seu vetor com o estado interno.
// Recebe: reserva original.
func copiarReserva(r protocolo.Reserva) protocolo.Reserva {
	r.Assentos = append([]protocolo.AssentoReservado(nil), r.Assentos...)
	return r
}

// Confirmar trava os trechos em ordem de ID e valida fora da trava geral. O registro final usa g.mu brevemente, revalida sessao, chave e horarios e publica todos os assentos juntos. Cancelamentos usam manutencao exclusiva.
// Recebe: sessão e pedido de reserva local.
func (g *GrafoItinerarios) Confirmar(sessaoID string, p protocolo.ReservaItinerario) (protocolo.Reserva, error) {
	if err := validarChave(p.Chave); err != nil {
		return protocolo.Reserva{}, err
	}
	if len(p.TrechosIDs) < 1 || len(p.TrechosIDs) > 20 {
		return protocolo.Reserva{}, fmt.Errorf("informe de 1 a 20 trechos em ordem")
	}

	g.manutencao.RLock()
	defer g.manutencao.RUnlock()
	estados, err := g.localizarTrechos(p.TrechosIDs)
	if err != nil {
		return protocolo.Reserva{}, err
	}
	liberar := travarTrechos(estados)
	defer liberar()
	g.mu.RLock()
	lendo := true
	defer func() {
		if lendo {
			g.mu.RUnlock()
		}
	}()
	u, err := g.autorizar(sessaoID, protocolo.Passageiro)
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
	g.mu.RUnlock()
	lendo = false
	usados := make(map[string]bool)
	cidades := make(map[string]bool)
	assentos := make([]protocolo.AssentoReservado, 0, len(p.TrechosIDs))
	var anterior protocolo.Trecho
	var total int64
	agora := g.agora()

	for _, id := range p.TrechosIDs {
		estado := estados[id]
		t := estado.trecho
		if usados[id] || t.Status != protocolo.Ativa || t.Assentos <= 0 || !partida(t).After(agora) {
			return protocolo.Reserva{}, fmt.Errorf("trecho inexistente, repetido, cancelado, sem vagas ou trecho já iniciado")
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
			_, ocupado := estado.ocupados[n]
			_, preparado := estado.preparados[n]

			if !ocupado && !preparado {
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
	g.mu.Lock()
	defer g.mu.Unlock()
	if anterior, ok := g.chaves[chave]; ok {
		if anterior.assinatura != sig {
			return protocolo.Reserva{}, fmt.Errorf("chave já utilizada com outros dados")
		}
		return copiarReserva(g.reservas[anterior.id]), nil
	}
	if _, err := g.autorizar(sessaoID, protocolo.Passageiro); err != nil {
		return protocolo.Reserva{}, err
	}
	for _, a := range assentos {
		if !g.disponivel(estados[a.Trecho.ID].trecho, g.agora()) {
			return protocolo.Reserva{}, fmt.Errorf("trecho indisponível ou já iniciado")
		}
	}
	r := protocolo.Reserva{ID: g.novoID("R"), Passageiro: u.Email, Status: protocolo.Ativa, CriadaEm: g.agora().UTC().Format(time.RFC3339), Assentos: assentos, PrecoTotal: float64(total) / 100}

	for i, a := range r.Assentos {
		t := g.trechos[a.Trecho.ID].trecho
		t.Assentos--
		g.trechos[t.ID].trecho = t
		g.trechos[t.ID].ocupados[a.Numero] = r.ID
		r.Assentos[i].Trecho = t
	}
	g.reservas[r.ID] = r
	g.chaves[chave] = repeticao{assinatura: sig, id: r.ID}
	return copiarReserva(r), nil
}

// ConsultarReservas retorna copias das reservas do passageiro autenticado.
// Recebe: sessão do passageiro.
func (g *GrafoItinerarios) ConsultarReservas(sessaoID string) ([]protocolo.Reserva, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	u, err := g.autorizar(sessaoID, protocolo.Passageiro)
	if err != nil {
		return nil, err
	}
	resultado := make([]protocolo.Reserva, 0)
	for _, r := range g.reservas {
		if r.Passageiro == u.Email {
			resultado = append(resultado, copiarReserva(r))
		}
	}
	sort.Slice(resultado, func(i, j int) bool {
		return resultado[i].ID < resultado[j].ID
	})
	return resultado, nil
}

// cancelarReserva devolve as vagas uma unica vez e mantem o registro cancelado no historico. Auxiliar interno: quem chama deve manter a trava de escrita.
// Recebe: entrada e sessão do passageiro em m.
func (g *GrafoItinerarios) cancelarReserva(id, motivo string) protocolo.Reserva {
	r := g.reservas[id]

	if r.Status == protocolo.Cancelada {
		return copiarReserva(r)
	}
	for _, a := range r.Assentos {
		t := g.trechos[a.Trecho.ID].trecho
		delete(g.trechos[t.ID].ocupados, a.Numero)
		t.Assentos++
		g.trechos[t.ID].trecho = t
	}
	r.Status = protocolo.Cancelada
	r.Motivo = motivo
	r.CanceladaEm = g.agora().UTC().Format(time.RFC3339)
	g.reservas[id] = r
	return copiarReserva(r)
}

// CancelarReserva confere o dono e o inicio do itinerario antes de cancelar a reserva inteira.
// Recebe: sessão do passageiro e ID da reserva.
func (g *GrafoItinerarios) CancelarReserva(sessaoID, id string) (protocolo.Reserva, error) {
	g.manutencao.Lock()
	defer g.manutencao.Unlock()
	g.mu.Lock()
	defer g.mu.Unlock()
	u, err := g.autorizar(sessaoID, protocolo.Passageiro)
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

// CancelarCarona valida motorista e horarios, cancela a oferta e as reservas ativas que dependem dela.
// Recebe: sessão do motorista e ID da carona.
func (g *GrafoItinerarios) CancelarCarona(sessaoID, id string) (protocolo.Carona, error) {
	g.manutencao.Lock()
	defer g.manutencao.Unlock()
	g.mu.Lock()
	defer g.mu.Unlock()
	u, err := g.autorizar(sessaoID, protocolo.Motorista)
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
		t := g.trechos[tid].trecho
		t.Status = protocolo.Cancelada
		g.trechos[tid].trecho = t
	}
	g.cancelarReservasDosTrechos(o.ids, "Carona "+id+" cancelada pelo motorista; itinerário inteiro cancelado.")
	return g.consultarCarona(id), nil
}

// cancelarReservasDosTrechos exige trava exclusiva e cancelamento ja validado. Coleta as reservas antes de devolver vagas, pois cancelarReserva altera ocupados. Uma reserva que usa varios trechos recebe apenas um cancelamento e um aviso.
// Recebe: IDs e motivo; exige cancelamento validado e trava exclusiva.
func (g *GrafoItinerarios) cancelarReservasDosTrechos(ids []string, motivo string) {
	reservas := make(map[string]bool)
	for _, id := range ids {
		for _, rid := range g.trechos[id].ocupados {
			reservas[rid] = true
		}
	}
	for rid := range reservas {
		g.notificar(g.cancelarReserva(rid, motivo))
	}
}

// CancelarTrecho Protege o estado com Lock e impede cancelar depois do inicio da carona ou de um itinerario ativo que utiliza o trecho. Se permitido, cancela o trecho e as reservas afetadas por inteiro, devolve as vagas, registra os avisos e recalcula o status da carona.
// Recebe: sessão do motorista e ID do trecho.
func (g *GrafoItinerarios) CancelarTrecho(sessaoID, id string) (protocolo.Carona, error) {
	g.manutencao.Lock()
	defer g.manutencao.Unlock()
	g.mu.Lock()
	defer g.mu.Unlock()
	u, err := g.autorizar(sessaoID, protocolo.Motorista)
	if err != nil {
		return protocolo.Carona{}, err
	}
	estado, ok := g.trechos[id]
	if !ok || estado.trecho.Motorista != u.Email {
		return protocolo.Carona{}, fmt.Errorf("trecho não encontrado para este motorista")
	}
	t := estado.trecho
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
	g.trechos[id].trecho = t
	g.cancelarReservasDosTrechos([]string{id}, "Trecho "+t.Origem+" -> "+t.Destino+" cancelado pelo motorista; itinerário inteiro cancelado.")

	o.status = protocolo.Cancelada
	for _, tid := range o.ids {
		if g.trechos[tid].trecho.Status == protocolo.Ativa {
			o.status = protocolo.Parcial
			break
		}
	}
	return g.consultarCarona(t.CaronaID), nil
}
