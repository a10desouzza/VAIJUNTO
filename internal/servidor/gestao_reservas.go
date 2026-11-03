// TEC502 - Problema 2. Autor: Arthur de Andrade Souza.
// Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
package servidor

import (
	"context"
	"fmt"
	"sort"
	"time"
	"vaijunto/internal/protocolo"
)

func (d *Distribuido) reservaOrigem(id string) protocolo.Reserva {
	d.mu.Lock()
	defer d.mu.Unlock()
	if op := d.origens[id]; op != nil {
		return copiarReserva(op.Reserva)
	}
	return protocolo.Reserva{}
}

func (d *Distribuido) atualizarOrigem(ctx context.Context, id string) {
	d.mu.Lock()
	op := copiaJSON(d.origens[id])
	reg := copiaJSON(d.registros[id])
	d.mu.Unlock()
	if op == nil || op.Reserva.Status != pendente || reg == nil || reg.Final == nil {
		return
	}
	c := *reg.Final
	rs := d.propagar(ctx, c)
	r := op.Reserva
	if c.Valor.Estado == "ABORTAR" {
		r.Status = protocolo.Cancelada
		r.Motivo = c.Valor.Motivo
	} else {
		for _, empresa := range c.Meta.Participantes {
			if rs[empresa].Reserva.ID == "" {
				return
			}
		}
		for i, a := range r.Assentos {
			for _, local := range rs[a.Trecho.EmpresaID].Reserva.Assentos {
				if local.Trecho.ID == a.Trecho.ID {
					r.Assentos[i].Numero = local.Numero
					r.Assentos[i].Trecho.Assentos = local.Trecho.Assentos
				}
			}
		}
		r.Status = protocolo.Ativa
		r.Motivo = ""
	}
	d.mu.Lock()
	if atual := d.origens[id]; atual != nil && atual.Reserva.Status == pendente {
		atual.Reserva = r
	}
	d.mu.Unlock()
	if err := d.g.SalvarBackup(); err != nil {
		d.g.logger.Printf("BACKUP transacao=%s erro=%v", id, err)
	}
}

func (d *Distribuido) atualizarOrigens(ctx context.Context) {
	d.mu.Lock()
	ids := make([]string, 0)
	for id, op := range d.origens {
		if op.Reserva.Status == pendente {
			ids = append(ids, id)
		}
	}
	d.mu.Unlock()
	for _, id := range ids {
		d.atualizarOrigem(ctx, id)
	}
}

func (d *Distribuido) consultar(sessao string) ([]protocolo.Reserva, error) {
	d.g.mu.RLock()
	u, err := d.g.autorizar(sessao, protocolo.Passageiro)
	d.g.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	locais, err := d.g.ConsultarReservas(sessao)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	for _, op := range d.origens {
		if op.Passageiro == u.Email {
			locais = append(locais, copiarReserva(op.Reserva))
		}
	}
	d.mu.Unlock()
	ctx, cancelar := context.WithTimeout(context.Background(), d.cliente.Timeout)
	defer cancelar()
	disponiveis := d.geracoes(ctx)
	for i, reserva := range locais {
		if reserva.Status == protocolo.Cancelada {
			continue
		}
		empresas := make(map[string]bool)
		for _, assento := range reserva.Assentos {
			id := assento.Trecho.EmpresaID
			if id != "" && disponiveis[id] == "" {
				empresas[id] = true
			}
		}
		for id := range empresas {
			locais[i].EmpresasIndisponiveis = append(locais[i].EmpresasIndisponiveis, id)
		}
		sort.Strings(locais[i].EmpresasIndisponiveis)
	}
	sort.Slice(locais, func(i, j int) bool {
		return locais[i].ID < locais[j].ID
	})
	return locais, nil
}

func (d *Distribuido) cancelar(sessao, id string) (protocolo.Reserva, error) {
	d.g.mu.RLock()
	u, err := d.g.autorizar(sessao, protocolo.Passageiro)
	d.g.mu.RUnlock()
	if err != nil {
		return protocolo.Reserva{}, err
	}
	d.mu.Lock()
	op := copiaJSON(d.origens[id])
	d.mu.Unlock()
	if op == nil {
		return d.g.CancelarReserva(sessao, id)
	}
	if op.Passageiro != u.Email {
		return protocolo.Reserva{}, fmt.Errorf("reserva não encontrada para este passageiro")
	}
	if err := d.cancelarGlobal(context.Background(), op.Meta, "Cancelada pelo passageiro."); err != nil {
		return protocolo.Reserva{}, err
	}
	return d.reservaOrigem(id), nil
}

func (d *Distribuido) cancelarGlobal(ctx context.Context, original metaTransacao, motivo string) error {
	liberar := travarOperacao(&d.chamadas, original.ID+":cancelar")
	defer liberar()
	ctx, cancelar := context.WithTimeout(ctx, 20*time.Second)
	defer cancelar()
	m := copiaJSON(original)
	m.ID = original.ID + ":cancelar"
	m.Tipo = "CANCELAR"
	m.Original = original.ID
	pedidos := make(map[string]protocolo.PedidoPreparacao)
	m.Hashes = make(map[string]string)
	for _, empresa := range m.Participantes {
		q := protocolo.PedidoPreparacao{TransacaoID: m.ID, EmpresaOrigem: m.Origem, PassageiroRef: "cancelamento", InicioItinerario: m.Inicio, Participantes: m.Participantes, TrechosIDs: []string{"cancelamento"}}
		pedidos[empresa] = q
		m.Hashes[empresa] = hashJSON(q)
	}
	d.mu.Lock()
	existente := copiaJSON(d.registros[m.ID])
	if existente != nil && existente.Final != nil && existente.Final.Valor.Estado == "ABORTAR" {
		// Uma recusa anterior nao impede tentar um novo cancelamento depois.
		d.relogio++
		m.ID = original.ID + ":c:" + hashJSON([]any{d.geracao, d.relogio})[:16]
		existente = nil
		for empresa, q := range pedidos {
			q.TransacaoID = m.ID
			pedidos[empresa] = q
			m.Hashes[empresa] = hashJSON(q)
		}
	}
	d.mu.Unlock()
	if err := d.g.SalvarBackup(); err != nil {
		return err
	}
	var cert certificado
	var err error
	if existente != nil && existente.Final != nil {
		cert = *existente.Final
	} else {
		v := d.prepararParticipantes(ctx, m, pedidos)
		if v.Estado == "CONFIRMAR" {
			v.Motivo = motivo
		}
		cert, err = d.decidir(ctx, m, v)
		if err != nil {
			return err
		}
	}
	rs := d.propagar(ctx, cert)
	if cert.Valor.Estado != "CONFIRMAR" {
		return fmt.Errorf("cancelamento recusado: %s", cert.Valor.Motivo)
	}
	for _, id := range m.Participantes {
		if _, ok := rs[id]; !ok {
			return fmt.Errorf("cancelamento decidido, aplicação pendente em %s; consulte novamente", id)
		}
	}
	return nil
}

// cancelarOferta resolve as reservas distribuidas antes de alterar a carona do motorista.
func (d *Distribuido) cancelarOferta(sessao, id string, trecho bool) (protocolo.Carona, error) {
	d.g.mu.RLock()
	u, err := d.g.autorizar(sessao, protocolo.Motorista)
	var ids []string
	if err == nil {
		if trecho {
			e := d.g.trechos[id]
			if e == nil || e.trecho.Motorista != u.Email {
				err = fmt.Errorf("trecho não encontrado")
			} else {
				ids = []string{id}
			}
		} else {
			o := d.g.caronas[id]
			if o == nil || o.motorista != u.Email {
				err = fmt.Errorf("carona não encontrada")
			} else {
				ids = append([]string(nil), o.ids...)
			}
		}
	}
	transacoes := make(map[string]bool)
	if err == nil {
		for _, tid := range ids {
			if len(d.g.trechos[tid].preparados) > 0 {
				err = fmt.Errorf("há uma preparação pendente; tente novamente após a decisão")
				break
			}
			for _, rid := range d.g.trechos[tid].ocupados {
				if tx := d.g.reservas[rid].TransacaoID; tx != "" {
					transacoes[tx] = true
				}
			}
		}
	}
	d.g.mu.RUnlock()
	if err != nil {
		return protocolo.Carona{}, err
	}
	for tx := range transacoes {
		d.mu.Lock()
		r := copiaJSON(d.registros[tx])
		d.mu.Unlock()
		if r == nil {
			return protocolo.Carona{}, fmt.Errorf("metadados da reserva indisponíveis")
		}
		if err := d.cancelarGlobal(context.Background(), r.Meta, "Oferta cancelada pelo motorista."); err != nil {
			return protocolo.Carona{}, err
		}
	}
	if trecho {
		return d.g.CancelarTrecho(sessao, id)
	}
	return d.g.CancelarCarona(sessao, id)
}

// CancelarTrecho Protege o estado com Lock e impede cancelar depois do inicio da carona ou de um
// itinerario ativo que utiliza o trecho. Se permitido, cancela o trecho e as reservas afetadas por
// inteiro, devolve as vagas, registra os avisos e recalcula o status da carona.
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
	/* Se restou um trecho ativo, a carona fica parcialmente cancelada. */
	o.status = protocolo.Cancelada
	for _, tid := range o.ids {
		if g.trechos[tid].trecho.Status == protocolo.Ativa {
			o.status = protocolo.Parcial
			break
		}
	}
	return g.consultarCarona(t.CaronaID), nil
}
