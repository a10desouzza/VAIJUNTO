// TEC502 - Problema 2. Autor: Arthur de Andrade Souza.
// Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
package cliente

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"vaijunto/internal/protocolo"
)

// publicar monta rota e trechos, mostra o resumo e envia apos a confirmacao do motorista.
func (m *menu) publicar() error {
	m.cabecalho("Publicar carona", "")
	n := m.numero("Quantidade de cidades da rota", 2, 21)
	p := protocolo.PublicacaoCarona{Rota: make([]string, n)}
	for i := range p.Rota {
		p.Rota[i] = m.cidade(fmt.Sprintf("Cidade %d", i+1), p.Rota[:i])
	}
	data := m.data()
	for m.err == nil {
		hora := m.texto("Horário de partida (HH:MM, fuso -03:00)")
		horario, err := time.Parse("2006-01-02T15:04-07:00", data+"T"+hora+"-03:00")
		if err == nil {
			p.DataHora = horario.Format(time.RFC3339)
			break
		}
		fmt.Fprintln(m.saida, "Horário inválido. Exemplo: 08:30.")
	}
	p.Assentos = m.numero("Assentos disponíveis", 1, 100)
	for i := 0; i < n-1 && m.err == nil; i++ {
		fmt.Fprintf(m.saida, "\nTrecho %d: %s -> %s\n", i+1, p.Rota[i], p.Rota[i+1])
		distancia := m.decimal("Distância deste trecho em km", 0.01, 100000)

		trecho := protocolo.OfertaTrecho{DistanciaKM: distancia, Preco: m.preco(), TempoViagem: m.numero("Tempo de viagem em minutos", 1, 10080)}
		if i < n-2 {
			trecho.TempoParada = m.numero("Parada após este trecho em minutos", 0, 10080)
		}
		p.Trechos = append(p.Trechos, trecho)
	}
	if m.err != nil {
		return m.err
	}
	m.cabecalho("Confirmar carona", strings.Join(p.Rota, " -> "))
	fmt.Fprintf(m.saida, "  Partida: %s\n  Assentos: %d\n", horarioLegivel(p.DataHora), p.Assentos)
	for i, t := range p.Trechos {
		fmt.Fprintf(m.saida, "  %s -> %s: %.2f km | %s por passageiro | %d min", p.Rota[i], p.Rota[i+1], t.DistanciaKM, dinheiro(t.Preco), t.TempoViagem)
		if i < len(p.Trechos)-1 {
			fmt.Fprintf(m.saida, " + %d min de parada", t.TempoParada)
		}
		fmt.Fprintln(m.saida)
	}
	if m.numero("Publicar? 1 = sim, 0 = voltar", 0, 1) != 1 {
		return nil
	}
	chave, err := novaChave()
	if err != nil {
		return err
	}
	p.Chave = chave
	var c protocolo.Carona
	if err := m.enviar(protocolo.AcaoPublicar, p, &c); err != nil {
		return err
	}
	fmt.Fprintf(m.saida, "Carona %s publicada com sucesso.\n", c.ID)
	return nil
}

// caronas consulta e mostra vagas e passageiros por trecho. Retorna a lista para selecao.
func (m *menu) caronas() ([]protocolo.Carona, error) {
	m.cabecalho("Minhas caronas", "")
	var caronas []protocolo.Carona
	if err := m.enviar(protocolo.AcaoCaronas, struct{}{}, &caronas); err != nil {
		return nil, err
	}
	if len(caronas) == 0 {
		fmt.Fprintln(m.saida, "Você ainda não publicou caronas.")
	}
	for i, c := range caronas {
		m.cabecalho(fmt.Sprintf("[%d] %s  |  %s", i+1, c.ID, c.Status), strings.Join(c.Rota, " -> "))
		fmt.Fprintf(m.saida, "  Partida: %s\n", horarioLegivel(c.DataHora))
		for j, t := range c.Trechos {
			fmt.Fprintf(m.saida, "  Trecho %d: %s -> %s | %s | %d/%d vagas | %s\n", j+1, t.Trecho.Origem, t.Trecho.Destino, t.Trecho.Status, t.Trecho.Assentos, t.Trecho.Capacidade, dinheiro(t.Trecho.Preco))
			for _, p := range t.Passageiros {
				fmt.Fprintf(m.saida, "     Passageiro: %s (%s)\n", p.Passageiro.Nome, p.Passageiro.Email)
			}
		}
	}
	return caronas, nil
}

// cancelarCarona seleciona uma oferta e pede confirmacao antes de solicitar o cancelamento.
func (m *menu) cancelarCarona() error {
	caronas, err := m.caronas()
	if err != nil || len(caronas) == 0 {
		return err
	}
	n := m.numero("Número da carona para cancelar (0 = voltar)", 0, len(caronas))
	if n == 0 {
		return nil
	}
	c := caronas[n-1]
	if c.Status == protocolo.Cancelada {
		fmt.Fprintln(m.saida, "Esta carona já foi cancelada.")
		return nil
	}
	fmt.Fprintln(m.saida, "Os itinerários reservados que usam esta carona serão cancelados por inteiro.")
	if m.numero("Confirmar cancelamento? 1 = sim, 0 = voltar", 0, 1) != 1 {
		return nil
	}
	if err := m.enviar(protocolo.AcaoCancelarCarona, protocolo.Identificador{ID: c.ID}, nil); err != nil {
		return err
	}
	fmt.Fprintln(m.saida, "Carona cancelada.")
	return nil
}

// cancelarTrecho consulta caronas, seleciona um trecho e confirma a solicitacao de cancelamento.
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

// buscar consulta itinerarios e permite reservar uma opcao enviando os IDs na ordem do percurso.
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

// entrar autentica e guarda a sessao localmente. Recusa uma conta de perfil diferente do cliente
// aberto.
func (m *menu) entrar() error {
	m.cabecalho("Login", "")
	p := protocolo.Credenciais{Email: m.email(), Senha: m.campo("Senha (visível no terminal)", nil, protocolo.ValidarSenha)}
	var sessao protocolo.Sessao
	if err := m.enviar(protocolo.AcaoEntrar, p, &sessao); err != nil {
		return err
	}
	if sessao.Usuario.Perfil != m.perfil {
		EnviarRequisicao(m.endereco, protocolo.Requisicao{Acao: protocolo.AcaoSair, SessaoID: sessao.ID, Dados: json.RawMessage(`{}`)})
		return fmt.Errorf("esta conta pertence ao perfil %s; abra o cliente correspondente", sessao.Usuario.Perfil)
	}
	m.sessaoID = sessao.ID
	m.nome = sessao.Usuario.Nome
	fmt.Fprintf(m.saida, "\nLogin realizado: %s\n", sessao.Usuario.Nome)
	return nil
}
