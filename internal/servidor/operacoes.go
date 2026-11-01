// Arquivo: internal/servidor/operacoes.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Concentra verificações de disponibilidade, início de viagem e permissão de cancelamento.
// Gerencia avisos destinados aos usuários e a marcação de notificações lidas.
// Impede alterar uma oferta por fora do protocolo quando existem transações protegidas.

package servidor

import (
	"fmt"
	"math"
	"time"

	"vaijunto/internal/protocolo"
)

// decimalValido rejeita negativos, NaN, infinitos, valores acima do limite ou com mais de duas
// casas decimais.
// Recebe: valor, limite float64.
// Retorna: bool.
func decimalValido(valor, limite float64) bool {
	return !math.IsNaN(valor) && !math.IsInf(valor, 0) && valor >= 0 && valor <= limite && math.Abs(valor*100-math.Round(valor*100)) < 0.000001
}

// iniciou compara a partida da carona com o relogio. No horario exato da partida ela ja iniciou.
// Recebe: o *oferta, agora time.Time.
// Retorna: bool.
func (g *GrafoItinerarios) iniciou(o *oferta, agora time.Time) bool {
	h, err := time.Parse(time.RFC3339, o.entrada.DataHora)
	return err != nil || !h.After(agora)
}

// disponivel exige trecho ativo, vagas e partida do trecho no futuro. Quem chama protege os mapas
// com trava.
// Recebe: t protocolo.Trecho, agora time.Time.
// Retorna: bool.
func (g *GrafoItinerarios) disponivel(t protocolo.Trecho, agora time.Time) bool {
	o, ok := g.caronas[t.CaronaID]
	return ok && o.status != protocolo.Cancelada && t.Status == protocolo.Ativa && t.Assentos > 0 && partida(t).After(agora)
}

// validarCancelamento impede cancelar conexoes de reservas ativas cujo primeiro trecho ja
// iniciou. Examina o itinerario inteiro, inclusive trechos de outros motoristas. Exige trava.
// Recebe: ids []string, agora time.Time.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
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

// ConsultarNotificacoes retorna uma copia dos avisos do usuario, incluindo os ja lidos.
// Recebe: sessaoID string.
// Retorna: ([]protocolo.Notificacao, error). Em falha, devolve error; em sucesso, erro nil.
func (g *GrafoItinerarios) ConsultarNotificacoes(sessaoID string) ([]protocolo.Notificacao, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	u, err := g.autorizar(sessaoID, "")
	if err != nil {
		return nil, err
	}
	return append([]protocolo.Notificacao{}, g.notificacoes[u.Email]...), nil
}

// LerNotificacao marca um aviso do proprio usuario como lido. Retorna erro se o ID nao lhe
// pertencer.
// Recebe: sessaoID, id string.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
func (g *GrafoItinerarios) LerNotificacao(sessaoID, id string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	u, err := g.autorizar(sessaoID, "")
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

// notificar registra em RAM um aviso da reserva cancelada. Quem chama ja mantem a trava de
// escrita.
// Recebe: r protocolo.Reserva.
// Retorna: nenhum valor; executa os efeitos descritos acima.
func (g *GrafoItinerarios) notificar(r protocolo.Reserva) {
	n := protocolo.Notificacao{ID: g.novoID("N"), ReservaID: r.ID, Mensagem: r.Motivo, CriadaEm: g.agora().UTC().Format(time.RFC3339)}
	g.notificacoes[r.Passageiro] = append(g.notificacoes[r.Passageiro], n)
}

// CancelarTrecho Protege o estado com Lock e impede cancelar depois do inicio da carona ou de um
// itinerario ativo que utiliza o trecho. Se permitido, cancela o trecho e as reservas afetadas por
// inteiro, devolve as vagas, registra os avisos e recalcula o status da carona.
// Recebe: sessaoID, id string.
// Retorna: (protocolo.Carona, error). Em falha, devolve error; em sucesso, erro nil.
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
