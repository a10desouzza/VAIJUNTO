/* ================================================================================================
 * internal/cliente/notificacoes.go - VaiJunto: sistema de caronas compartilhadas
 * Autor: Arthur Souza
 *
 * Consulta de avisos e formulario de cancelamento de trecho.
 * Os avisos sao buscados pelo cliente; nao existe envio espontaneo pelo servidor.
 *
 * DIVISAO DE RESPONSABILIDADES:
 * O cliente coleta entradas e exibe respostas; o servidor decide permissoes, disponibilidade e
 * alteracoes nas reservas.
 * ================================================================================================ */

package cliente

import (
	"encoding/json"
	"fmt"
	"vaijunto/internal/protocolo"
)

/* notificacoes
 *
 * Recebe: todas: true inclui lidas; false mostra apenas novas; m: token, endereco e destino de
 * saida.
 *
 * O que faz: Consulta os avisos do usuario, mostra cada aviso selecionado e so depois envia a
 * confirmacao
 * de leitura dos novos. Se essa confirmacao falhar, o aviso pode aparecer novamente.
 *
 * Retorna: nil ao concluir; error de comunicacao ou operacao. A falha na consulta pode limpar o
 * token local.
 */
func (m *menu) notificacoes(todas bool) error {
	resp, err := EnviarRequisicao(m.endereco, protocolo.Requisicao{Acao: protocolo.AcaoNotificacoes, Token: m.token, Dados: json.RawMessage(`{}`)})
	if err != nil {
		return err
	}
	if resp.Status != protocolo.Sucesso {
		m.token = ""
		return fmt.Errorf("%s", resp.Mensagem)
	}
	dados, err := json.Marshal(resp.Dados)
	if err != nil {
		return err
	}
	var avisos []protocolo.Notificacao
	if err := json.Unmarshal(dados, &avisos); err != nil {
		return err
	}
	if todas && len(avisos) == 0 {
		fmt.Fprintln(m.saida, "Nenhuma notificação.")
	}
	for _, aviso := range avisos {
		if aviso.Lida && !todas {
			continue
		}
		fmt.Fprintf(m.saida, "\nAviso da reserva %s (%s):\n%s\n", aviso.ReservaID, horarioLegivel(aviso.CriadaEm), aviso.Mensagem)
		/* Confirma a leitura somente depois de mostrar o aviso no terminal. */
		if !aviso.Lida {
			raw, _ := json.Marshal(protocolo.Identificador{ID: aviso.ID})
			confirmacao, err := EnviarRequisicao(m.endereco, protocolo.Requisicao{Acao: protocolo.AcaoLerNotificacao, Token: m.token, Dados: raw})
			if err != nil {
				return err
			}
			if confirmacao.Status != protocolo.Sucesso {
				return fmt.Errorf("%s", confirmacao.Mensagem)
			}
		}
	}
	return nil
}

/* cancelarTrecho
 *
 * Recebe: Nenhum argumento explicito; usa m para consultar caronas e selecionar o trecho.
 *
 * O que faz: consulta caronas, seleciona um trecho e confirma a solicitacao de cancelamento.
 *
 * Retorna: nil ao cancelar, voltar ou encontrar trecho ja cancelado; error de consulta ou envio.
 */
func (m *menu) cancelarTrecho() error {
	caronas, err := m.caronas()
	if err != nil || len(caronas) == 0 {
		return err
	}
	n := m.numero("Número da carona (0 = voltar)", 0, len(caronas))
	if n == 0 {
		return nil
	}
	carona := caronas[n-1]
	n = m.numero("Número do trecho para cancelar (0 = voltar)", 0, len(carona.Trechos))
	if n == 0 {
		return nil
	}
	trecho := carona.Trechos[n-1].Trecho
	if trecho.Status == protocolo.Cancelada {
		fmt.Fprintln(m.saida, "Trecho já cancelado.")
		return nil
	}
	fmt.Fprintln(m.saida, "As reservas que dependem deste trecho serão canceladas por inteiro.")
	if m.numero("Confirmar? 1 = sim, 0 = voltar", 0, 1) != 1 {
		return nil
	}
	if err := m.enviar(protocolo.AcaoCancelarTrecho, protocolo.Identificador{ID: trecho.ID}, nil); err != nil {
		return err
	}
	fmt.Fprintln(m.saida, "Trecho cancelado. Consulte as caronas para verificar as vagas.")
	return nil
}
