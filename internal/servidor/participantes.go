// Autor: Arthur de Andrade Souza.
// Matéria: TEC502 - TP01 - Concorrência e Conectividade - 2026.2.
// Declaração de não plágio: Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
// Funcionalidade: Valida pedidos e decisões do 2PC e solicita a preparação e a aplicação idempotente dos efeitos locais.
package servidor

import (
	"crypto/hmac"
	"fmt"
	"time"
	"vaijunto/internal/protocolo"
)

// validarMeta confere participantes, origem e a geracao do estado local.
// Recebe: metadados e identidade local d.
func (d *Distribuido) validarMeta(m metaTransacao) error {
	if m.ID == "" || len(m.ID) > 128 || (m.Tipo != "RESERVAR" && m.Tipo != "CANCELAR") || !contem(d.membros, m.Origem) {
		return fmt.Errorf("metadados inválidos")
	}
	if m.Geracoes[d.id] != d.geracao {
		return fmt.Errorf("geração do processo perdida; operação antiga não pode ser reaplicada")
	}
	if _, err := time.Parse(time.RFC3339, m.Inicio); err != nil {
		return fmt.Errorf("início inválido")
	}
	if len(m.Participantes) < 1 || len(m.Participantes) > 3 {
		return fmt.Errorf("participantes inválidos")
	}
	vistos := make(map[string]bool)
	for _, id := range m.Participantes {
		if vistos[id] || !contem(d.membros, id) || m.Hashes[id] == "" {
			return fmt.Errorf("participante inválido")
		}
		vistos[id] = true
	}
	if m.Tipo == "CANCELAR" && m.Original == "" {
		return fmt.Errorf("transação original obrigatória")
	}
	return nil
}

// validarDecisao exige o recibo de todos os donos antes de aceitar uma confirmacao.
// Recebe: metadados e decisão proposta.
func (d *Distribuido) validarDecisao(m metaTransacao, v decisao) error {
	if v.Estado == "ABORTAR" {
		return nil
	}
	if v.Estado != "CONFIRMAR" {
		return fmt.Errorf("decisão inválida")
	}
	for _, id := range m.Participantes {
		if !hmac.Equal([]byte(v.Recibos[id]), []byte(d.recibo(m, id))) {
			return fmt.Errorf("faltou preparação de %s", id)
		}
	}
	return nil
}

// preparar registra a operacao e separa somente os assentos da empresa local.
// Recebe: metadados e pedido da parte local.
func (d *Distribuido) preparar(p prepararRPC) (resultadoRPC, error) {
	if err := d.validarMeta(p.Meta); err != nil {
		return resultadoRPC{}, err
	}
	mesmaTransacao := p.Pedido.TransacaoID == p.Meta.ID && p.Pedido.EmpresaOrigem == p.Meta.Origem
	mesmoItinerario := p.Pedido.InicioItinerario == p.Meta.Inicio && hashJSON(p.Pedido.Participantes) == hashJSON(p.Meta.Participantes)
	pedidoIntegro := p.Meta.Hashes[d.id] == hashJSON(p.Pedido)
	if !contem(p.Meta.Participantes, d.id) || !mesmaTransacao || !mesmoItinerario || !pedidoIntegro {
		return resultadoRPC{}, fmt.Errorf("pedido diverge dos metadados")
	}
	liberar := travarOperacao(&d.locais, p.Meta.ID)
	defer liberar()
	d.mu.Lock()
	r := d.registros[p.Meta.ID]
	if r == nil {
		r = &registroTransacao{Meta: p.Meta, Criado: time.Now()}
		d.registros[p.Meta.ID] = r
	}
	if hashJSON(r.Meta) != hashJSON(p.Meta) || r.Final != nil {
		d.mu.Unlock()
		return resultadoRPC{}, fmt.Errorf("transação divergente ou já decidida")
	}
	d.mu.Unlock()
	if p.Meta.Tipo == "CANCELAR" {
		d.g.mu.RLock()
		t := d.g.transacoes[p.Meta.Original]
		ok := t != nil && t.estado == protocolo.TransacaoConfirmada
		d.g.mu.RUnlock()
		inicio, _ := time.Parse(time.RFC3339, p.Meta.Inicio)
		if !ok || !inicio.After(d.g.agora()) {
			return resultadoRPC{}, fmt.Errorf("reserva não encontrada ou viagem iniciada")
		}
	} else {
		r, err := d.g.prepararTransacao(p.Pedido)
		if err != nil {
			return resultadoRPC{}, err
		}
		if r.Estado != protocolo.TransacaoPreparada {
			return resultadoRPC{}, fmt.Errorf("transação não está preparada")
		}
	}
	return resultadoRPC{Recibo: d.recibo(p.Meta, d.id)}, nil
}

// resultadoLocal obtem uma copia da reserva local associada a transacao, se ela existir.
// Recebe: ID da transação local.
func (d *Distribuido) resultadoLocal(id string) resultadoRPC {
	d.g.mu.RLock()
	defer d.g.mu.RUnlock()
	t := d.g.transacoes[id]
	if t == nil {
		return resultadoRPC{}
	}
	return resultadoRPC{Reserva: copiarReserva(d.g.reservas[t.reservaID])}
}

// finalizar aplica a decisao autenticada do coordenador uma unica vez.
// Recebe: decisão final autenticada.
func (d *Distribuido) finalizar(c decisaoFinal) (resultadoRPC, error) {
	if err := d.validarMeta(c.Meta); err != nil {
		return resultadoRPC{}, err
	}
	if err := d.validarDecisao(c.Meta, c.Valor); err != nil {
		return resultadoRPC{}, err
	}
	if !hmac.Equal([]byte(c.Assinatura), []byte(d.assinar([]any{c.Meta, c.Valor}))) {
		return resultadoRPC{}, fmt.Errorf("decisao sem autenticacao do coordenador")
	}
	liberar := travarOperacao(&d.locais, c.Meta.ID)
	defer liberar()
	d.mu.Lock()
	r := d.registros[c.Meta.ID]
	if r == nil {
		r = &registroTransacao{Meta: c.Meta, Criado: time.Now()}
		d.registros[c.Meta.ID] = r
	}
	if hashJSON(r.Meta) != hashJSON(c.Meta) || r.Final != nil && hashJSON(r.Final.Valor) != hashJSON(c.Valor) {
		d.mu.Unlock()
		return resultadoRPC{}, fmt.Errorf("decisão conflitante")
	}
	if r.Aplicado {
		d.mu.Unlock()
		id := c.Meta.ID
		if c.Meta.Tipo == "CANCELAR" {
			id = c.Meta.Original
		}
		return d.resultadoLocal(id), nil
	}
	cp := copiaJSON(c)
	r.Final = &cp
	d.mu.Unlock()
	var reserva protocolo.Reserva
	var err error
	if contem(c.Meta.Participantes, d.id) {
		if c.Meta.Tipo == "CANCELAR" {
			if c.Valor.Estado == "CONFIRMAR" {
				reserva, err = d.g.cancelarTransacaoConfirmada(c.Meta.Original, c.Valor.Motivo)
			}
		} else if c.Valor.Estado == "CONFIRMAR" {
			reserva, err = d.g.confirmarTransacao(c.Meta.ID)
		} else {
			d.g.mu.RLock()
			existe := d.g.transacoes[c.Meta.ID] != nil
			d.g.mu.RUnlock()
			if existe {
				_, err = d.g.abortarTransacao(c.Meta.ID, c.Valor.Motivo)
			}
		}
	}
	if err != nil {
		return resultadoRPC{}, err
	}
	d.mu.Lock()
	r.Aplicado = true
	var aviso *protocolo.Reserva
	if c.Meta.Tipo == "CANCELAR" && c.Valor.Estado == "CONFIRMAR" {
		if op := d.origens[c.Meta.Original]; op != nil && op.Reserva.Status != protocolo.Cancelada {
			op.Reserva.Status = protocolo.Cancelada
			op.Reserva.Motivo = c.Valor.Motivo
			op.Reserva.CanceladaEm = time.Now().UTC().Format(time.RFC3339)
			cp := copiarReserva(op.Reserva)
			aviso = &cp
		}
	}
	d.mu.Unlock()
	if aviso != nil {
		d.g.mu.Lock()
		d.g.notificar(*aviso)
		d.g.mu.Unlock()
	}
	return resultadoRPC{Reserva: reserva}, nil
}
