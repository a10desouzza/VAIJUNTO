// internal/cliente/passageiro.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Busca de itinerarios, confirmacao, consulta e cancelamento de reservas.

package cliente

import (
	"fmt"
	"vaijunto/internal/protocolo"
)

// buscar: consulta itinerarios e permite reservar uma opcao enviando os IDs na ordem do percurso.
// Retorna: nil ao reservar, voltar ou nao encontrar opcoes; error de entrada, chave ou requisicao.
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
	if len(busca.Itinerarios) == 0 {
		fmt.Fprintln(m.saida, "Nenhum itinerário disponível para essa data.")
		return nil
	}
	for i, it := range busca.Itinerarios {
		m.cabecalho(fmt.Sprintf("Opção %d  |  %s", i+1, dinheiro(it.PrecoTotal)), fmt.Sprintf("%d minutos  |  %d trecho(s)", it.DuracaoTotal, len(it.Trechos)))
		for _, t := range it.Trechos {
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
	p := protocolo.ReservaItinerario{Chave: chave, TrechosIDs: make([]string, 0, len(it.Trechos))}
	for _, t := range it.Trechos {
		p.TrechosIDs = append(p.TrechosIDs, t.ID)
	}
	var reserva protocolo.Reserva
	if err := m.enviar(protocolo.AcaoConfirmar, p, &reserva); err != nil {
		return err
	}
	fmt.Fprintf(m.saida, "Reserva %s confirmada!\n", reserva.ID)
	m.mostrarReserva(reserva)
	return nil
}

// mostrarReserva: mostra status, valor e percurso. Nao apresenta uma escolha de assento numerado.
func (m *menu) mostrarReserva(r protocolo.Reserva) {
	fmt.Fprintf(m.saida, "  %s | %s | %s\n", r.ID, r.Status, dinheiro(r.PrecoTotal))
	for _, a := range r.Assentos {
		fmt.Fprintf(m.saida, "  %s -> %s\n    %s | 1 vaga reservada\n", a.Trecho.Origem, a.Trecho.Destino, horarioLegivel(a.Trecho.DataHora))
	}
	if r.Motivo != "" {
		fmt.Fprintln(m.saida, "  ", r.Motivo)
	}
}

// reservas: consulta e apresenta as reservas do passageiro. Retorna a lista para as outras
// operacoes. Retorna: Lista de reservas exibidas e nil; nil e error se a consulta falhar.
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

// cancelarReserva: seleciona a reserva e confirma o cancelamento de todos os seus trechos.
// Retorna: nil ao cancelar ou voltar; error de consulta ou cancelamento.
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
