// internal/servidor/operacoes.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Regras de horario, cancelamento e notificacoes.
// O inicio e determinado pela partida programada, sem GPS ou confirmacao de embarque.

package servidor

import (
	"fmt"
	"math"
	"time"
	"vaijunto/internal/protocolo"
)

// decimalValido: rejeita negativos, NaN, infinitos, valores acima do limite ou com mais de duas
// casas decimais. Retorna: true para numero finito, nao negativo, dentro do limite e com ate duas
// casas; false caso contrario.
func decimalValido(valor, limite float64) bool {
	return !math.IsNaN(valor) && !math.IsInf(valor, 0) && valor >= 0 && valor <= limite && math.Abs(valor*100-math.Round(valor*100)) < 0.000001
}

// iniciou: compara a partida da carona com o relogio. No horario exato da partida ela ja iniciou.
// Retorna: true se a partida chegou, passou ou esta invalida; false se ainda esta no futuro.
func (g *GrafoItinerarios) iniciou(o *oferta, agora time.Time) bool {
	h, err := time.Parse(time.RFC3339, o.entrada.DataHora)
	return err != nil || !h.After(agora)
}

// disponivel: exige trecho ativo, vagas e partida do trecho no futuro. Quem chama protege os mapas
// com trava. Retorna: true quando o trecho nao iniciou e o trecho esta ativo e com vagas; false
// caso contrario.
func (g *GrafoItinerarios) disponivel(t protocolo.Trecho, agora time.Time) bool {
	o, ok := g.caronas[t.CaronaID]
	return ok && o.status != protocolo.Cancelada && t.Status == protocolo.Ativa && t.Assentos > 0 && partida(t).After(agora)
}

// validarCancelamento: impede cancelar conexoes de reservas ativas cujo primeiro trecho ja
// iniciou. Examina o itinerario inteiro, inclusive trechos de outros motoristas. Exige trava.
// Retorna: error se algum trecho pertence a uma reserva ativa cujo percurso iniciou; nil se nao ha
// esse bloqueio.
func (g *GrafoItinerarios) validarCancelamento(ids []string, agora time.Time) error {
	selecionados := make(map[string]bool, len(ids))
	for _, id := range ids {
		selecionados[id] = true
	}
	/* Basta o PRIMEIRO trecho da reserva ter iniciado para proteger suas conexoes futuras. */
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

// ConsultarNotificacoes: retorna uma copia dos avisos do usuario, incluindo os ja lidos. Retorna:
// Copia da lista de notificacoes, incluindo lidas, ou error de autorizacao.
func (g *GrafoItinerarios) ConsultarNotificacoes(sessaoID string) ([]protocolo.Notificacao, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	u, err := g.autorizar(sessaoID, "")
	if err != nil {
		return nil, err
	}
	return append([]protocolo.Notificacao{}, g.notificacoes[u.Email]...), nil
}

// LerNotificacao: marca um aviso do proprio usuario como lido. Retorna erro se o ID nao lhe
// pertencer. Retorna: nil se o aviso do usuario foi marcado; error de sessao ou aviso nao
// encontrado.
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

// notificar: registra em RAM um aviso da reserva cancelada. Quem chama ja mantem a trava de
// escrita.
func (g *GrafoItinerarios) notificar(r protocolo.Reserva) {
	n := protocolo.Notificacao{ID: g.novoID("N"), ReservaID: r.ID, Mensagem: r.Motivo, CriadaEm: g.agora().UTC().Format(time.RFC3339)}
	g.notificacoes[r.Passageiro] = append(g.notificacoes[r.Passageiro], n)
}

// CancelarTrecho: Protege o estado com Lock e impede cancelar depois do inicio da carona ou de um
// itinerario ativo que utiliza o trecho. Se permitido, cancela o trecho e as reservas afetadas por
// inteiro, devolve as vagas, registra os avisos e recalcula o status da carona. Retorna: Carona
// com status e trechos atualizados, ou error de autorizacao, propriedade ou horario.
func (g *GrafoItinerarios) CancelarTrecho(sessaoID, id string) (protocolo.Carona, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	u, err := g.autorizar(sessaoID, protocolo.Motorista)
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
	g.cancelarReservasDosTrechos([]string{id}, "Trecho "+t.Origem+" -> "+t.Destino+" cancelado pelo motorista; itinerário inteiro cancelado.")
	/* Se restou um trecho ativo, a carona fica parcialmente cancelada. */
	o.status = protocolo.Cancelada
	for _, tid := range o.ids {
		if g.trechos[tid].Status == protocolo.Ativa {
			o.status = protocolo.Parcial
			break
		}
	}
	return g.consultarCarona(t.CaronaID), nil
}
