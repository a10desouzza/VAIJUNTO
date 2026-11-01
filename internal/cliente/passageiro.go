// Arquivo: internal/cliente/passageiro.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Permite buscar, reservar, consultar e cancelar itinerários pelo terminal.
// Apresenta as empresas dos trechos, o estado da compra e os avisos de cancelamento.
// Encaminha referências e chaves ao servidor sem alterar os assentos no cliente.

package cliente

import (
	"encoding/json"
	"fmt"
	"strings"

	"vaijunto/internal/protocolo"
)

// buscar consulta itinerarios e permite reservar uma opcao enviando os IDs na ordem do percurso.
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: nenhum parametro.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
func (m *menu) buscar() error {
	m.cabecalho("Buscar itinerário", "")
	b := protocolo.BuscaItinerario{Origem: m.cidade("Origem", nil)}
	b.Destino = m.cidade("Destino", []string{b.Origem})
	b.Data = m.data()
	fmt.Fprintln(m.saida, "Resultados em ordem de horário de partida.")
	var busca protocolo.ResultadoBusca
	if err := m.enviar(protocolo.AcaoBuscar, b, &busca); err != nil {
		return err
	}
	if busca.Limitada {
		fmt.Fprintln(m.saida, "Busca limitada: podem existir outras opções além das apresentadas.")
	}
	if len(busca.Indisponiveis) > 0 {
		fmt.Fprintf(m.saida, "Consulta parcial: empresas indisponíveis: %v\n", busca.Indisponiveis)
	}
	if len(busca.Itinerarios) == 0 {
		fmt.Fprintln(m.saida, "Nenhum itinerário disponível para essa data.")
		return nil
	}
	for i, it := range busca.Itinerarios {
		m.cabecalho(fmt.Sprintf("Opção %d  |  %s", i+1, dinheiro(it.PrecoTotal)), fmt.Sprintf("%d minutos  |  %d trecho(s)", it.DuracaoTotal, len(it.Trechos)))
		for _, t := range it.Trechos {
			if t.EmpresaID != "" {
				fmt.Fprintf(m.saida, "  Empresa: %s\n", t.EmpresaID)
			}
			fmt.Fprintf(m.saida, "  %s -> %s\n    %s | %d vagas\n    Motorista: %s\n", t.Origem, t.Destino, horarioLegivel(t.DataHora), t.Assentos, t.Motorista)
		}
	}
	n := m.numero("Número do itinerário para reservar (0 = voltar)", 0, len(busca.Itinerarios))
	if n == 0 {
		return nil
	}
	it := busca.Itinerarios[n-1]
	fmt.Fprintf(m.saida, "  Total: %s para uma pessoa.\n", dinheiro(it.PrecoTotal))
	if m.numero("Confirmar reserva? 1 = sim, 0 = voltar", 0, 1) != 1 {
		return nil
	}
	chave, err := novaChave()
	if err != nil {
		return err
	}
	// envia a chave e os IDs dos trechos na ordem do percurso para confirmar a reserva
	p := protocolo.ReservaItinerario{Chave: chave, TrechosIDs: make([]string, 0, len(it.Trechos))}
	for _, t := range it.Trechos {
		if t.EmpresaID != "" {
			p.Trechos = append(p.Trechos, protocolo.ReferenciaTrecho{EmpresaID: t.EmpresaID, TrechoID: t.ID})
		} else {
			p.TrechosIDs = append(p.TrechosIDs, t.ID)
		}
	}
	var reserva protocolo.Reserva

	if err := m.enviar(protocolo.AcaoConfirmar, p, &reserva); err != nil {
		return err
	}
	if reserva.Status == protocolo.Ativa {
		fmt.Fprintf(m.saida, "Reserva %s confirmada!\n", reserva.ID)
	} else {
		fmt.Fprintf(m.saida, "Reserva %s: %s. Consulte Minhas reservas para acompanhar.\n", reserva.ID, reserva.Status)
	}
	m.mostrarReserva(reserva)
	return nil
}

// mostrarReserva mostra status, valor e percurso. Nao apresenta uma escolha de assento numerado.
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: r protocolo.Reserva.
// Retorna: nenhum valor; executa os efeitos descritos acima.
func (m *menu) mostrarReserva(r protocolo.Reserva) {
	fmt.Fprintf(m.saida, "  %s | %s | %s\n", r.ID, r.Status, dinheiro(r.PrecoTotal))
	for _, a := range r.Assentos {
		fmt.Fprintf(m.saida, "  %s -> %s\n    %s\n", a.Trecho.Origem, a.Trecho.Destino, horarioLegivel(a.Trecho.DataHora))
		if r.Status == protocolo.Ativa {
			fmt.Fprintln(m.saida, "    1 vaga reservada")
		}
	}
	if len(r.EmpresasIndisponiveis) > 0 {
		fmt.Fprintf(m.saida, "  Empresas sem resposta: %s. A indisponibilidade nao cancela a reserva.\n", strings.Join(r.EmpresasIndisponiveis, ", "))
	}
	if r.Motivo != "" {
		fmt.Fprintln(m.saida, "  ", r.Motivo)
	}
}

// reservas consulta e apresenta as reservas do passageiro. Retorna a lista para as outras
// operacoes.
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: nenhum parametro.
// Retorna: ([]protocolo.Reserva, error). Em falha, devolve error; em sucesso, erro nil.
func (m *menu) reservas() ([]protocolo.Reserva, error) {
	m.cabecalho("Minhas reservas", "")
	var reservas []protocolo.Reserva
	if err := m.enviar(protocolo.AcaoReservas, struct{}{}, &reservas); err != nil {
		return nil, err
	}
	if len(reservas) == 0 {
		fmt.Fprintln(m.saida, "Você ainda não tem reservas.")
	}
	for i, r := range reservas {
		m.cabecalho(fmt.Sprintf("Reserva %d", i+1), r.Status)
		m.mostrarReserva(r)
	}
	return reservas, nil
}

// cancelarReserva seleciona a reserva e confirma o cancelamento de todos os seus trechos.
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: nenhum parametro.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
func (m *menu) cancelarReserva() error {
	reservas, err := m.reservas()
	if err != nil || len(reservas) == 0 {
		return err
	}
	n := m.numero("Número da reserva para cancelar (0 = voltar)", 0, len(reservas))
	if n == 0 {
		return nil
	}
	r := reservas[n-1]
	if r.Status == protocolo.Cancelada {
		fmt.Fprintln(m.saida, "Esta reserva já foi cancelada.")
		return nil
	}
	if m.numero("Cancelar todos os trechos? 1 = sim, 0 = voltar", 0, 1) != 1 {
		return nil
	}
	if err := m.enviar(protocolo.AcaoCancelarReserva, protocolo.Identificador{ID: r.ID}, nil); err != nil {
		return err
	}
	fmt.Fprintln(m.saida, "Reserva cancelada. Os assentos foram devolvidos.")
	return nil
}

// notificacoes Consulta os avisos do usuario, mostra cada aviso selecionado e so depois envia a
// confirmacao de leitura dos novos. Se essa confirmacao falhar, o aviso pode aparecer novamente.
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: todas bool.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
func (m *menu) notificacoes(todas bool) error {
	resp, err := EnviarRequisicao(m.endereco, protocolo.Requisicao{Acao: protocolo.AcaoNotificacoes, SessaoID: m.sessaoID, Dados: json.RawMessage(`{}`)})
	if err != nil {
		return err
	}
	// se servidor respondeu algo diferente de sucesso, sessao pode ter ficado inativa
	if resp.Status != protocolo.Sucesso {
		m.sessaoID = ""
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
			confirmacao, err := EnviarRequisicao(m.endereco, protocolo.Requisicao{Acao: protocolo.AcaoLerNotificacao, SessaoID: m.sessaoID, Dados: raw})
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
