// internal/servidor/reservas.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Confirmacao e cancelamento de reservas.
// PONTO-CHAVE: verificar todos os trechos antes de descontar vagas, mantendo a mesma trava.

package servidor

import (
	"fmt"
	"sort"
	"time"
	"vaijunto/internal/protocolo"
)

// copiarReserva: copia a lista de assentos para a resposta nao compartilhar seu vetor com o estado
// interno. Retorna: Copia da reserva com vetor de assentos separado do original.
func copiarReserva(r protocolo.Reserva) protocolo.Reserva {
	r.Assentos = append([]protocolo.AssentoReservado(nil), r.Assentos...)
	return r
}

// Confirmar: Adquire a trava exclusiva, autoriza a sessao e verifica repeticao. Confere todos os
// trechos, suas vagas, horarios e conexoes antes de alterar o estado. So entao ocupa uma vaga por
// trecho, registra a reserva e a chave. O defer libera a trava antes da resposta seguir pela rede.
// Retorna: Reserva completa ou reserva existente da mesma chave; error sem descontar vagas se a
// verificacao falhar.
func (g *GrafoItinerarios) Confirmar(sessaoID string, p protocolo.ReservaItinerario) (protocolo.Reserva, error) {
	if err := validarChave(p.Chave); err != nil {
		return protocolo.Reserva{}, err
	}
	if len(p.TrechosIDs) < 1 || len(p.TrechosIDs) > 20 {
		return protocolo.Reserva{}, fmt.Errorf("informe de 1 a 20 trechos em ordem")
	}
	/* A mesma trava cobre verificacao e alteracao. O defer libera inclusive nos retornos de erro. */
	g.mu.Lock()
	defer g.mu.Unlock()
	u, err := g.autorizar(sessaoID, protocolo.Passageiro)
	if err != nil {
		return protocolo.Reserva{}, err
	}
	chave := u.Email + "|reservar|" + p.Chave
	sig := assinatura(p.TrechosIDs)
	/* Resposta perdida: a mesma chave e os mesmos dados devolvem a reserva ja criada. */
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
	/* ===================== Fase 1: somente verificacao =====================
	 * Ainda nao descontamos vagas. Se algum trecho falhar, o estado fica como estava. */
	for _, id := range p.TrechosIDs {
		t, ok := g.trechos[id]
		if !ok || usados[id] || !g.disponivel(t, agora) {
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
	/* ===================== Fase 2: confirmar todos os trechos =====================
	 * Todos passaram na verificacao e nenhuma outra escrita entrou durante a trava. */
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

// ConsultarReservas: retorna copias das reservas do passageiro autenticado. Retorna: Lista de
// copias das reservas desse passageiro, ordenada por ID, ou error de autorizacao.
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
	sort.Slice(resultado, func(i, j int) bool { return resultado[i].ID < resultado[j].ID })
	return resultado, nil
}

// cancelarReserva: devolve as vagas uma unica vez e mantem o registro cancelado no historico.
// Auxiliar interno: quem chama deve manter a trava de escrita. Retorna: Copia da reserva
// cancelada, ou do registro ja cancelado sem devolver vagas novamente.
func (g *GrafoItinerarios) cancelarReserva(id, motivo string) protocolo.Reserva {
	r := g.reservas[id]
	/* Impede que um cancelamento repetido aumente as vagas pela segunda vez. */
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

// CancelarReserva: confere o dono e o inicio do itinerario antes de cancelar a reserva inteira.
// Retorna: Reserva cancelada ou error se nao pertencer ao passageiro ou se o itinerario ja
// iniciou.
func (g *GrafoItinerarios) CancelarReserva(sessaoID, id string) (protocolo.Reserva, error) {
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

// CancelarCarona: valida motorista e horarios, cancela a oferta e as reservas ativas que dependem
// dela. Retorna: Carona cancelada ou error de propriedade/horario; repetir um cancelamento
// concluido devolve a carona.
func (g *GrafoItinerarios) CancelarCarona(sessaoID, id string) (protocolo.Carona, error) {
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
	/* Outra carona pode ter iniciado o itinerario de um passageiro desta oferta. */
	if err := g.validarCancelamento(o.ids, agora); err != nil {
		return protocolo.Carona{}, err
	}
	o.status = protocolo.Cancelada
	for _, tid := range o.ids {
		t := g.trechos[tid]
		t.Status = protocolo.Cancelada
		g.trechos[tid] = t
	}
	g.cancelarReservasDosTrechos(o.ids, "Carona "+id+" cancelada pelo motorista; itinerário inteiro cancelado.")
	return g.consultarCarona(id), nil
}

// cancelarReservasDosTrechos exige trava exclusiva e cancelamento ja validado.
// Coleta as reservas antes de devolver vagas, pois cancelarReserva altera ocupados.
// Uma reserva que usa varios trechos recebe apenas um cancelamento e um aviso.
func (g *GrafoItinerarios) cancelarReservasDosTrechos(ids []string, motivo string) {
	reservas := make(map[string]bool)
	for _, id := range ids {
		for _, rid := range g.ocupados[id] {
			reservas[rid] = true
		}
	}
	for rid := range reservas {
		g.notificar(g.cancelarReserva(rid, motivo))
	}
}
