// Arquivo: internal/cliente/motorista.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Reúne os formulários de publicação, consulta e cancelamento de caronas e trechos.
// Envia os dados do motorista à empresa escolhida e apresenta vagas e passageiros.
// As permissões e regras finais de cada operação são verificadas pelo servidor.

package cliente

import (
	"fmt"
	"strings"
	"time"

	"vaijunto/internal/protocolo"
)

// publicar monta rota e trechos, mostra o resumo e envia apos a confirmacao do motorista.
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: nenhum parametro.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
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
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: nenhum parametro.
// Retorna: ([]protocolo.Carona, error). Em falha, devolve error; em sucesso, erro nil.
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
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: nenhum parametro.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
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
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: nenhum parametro.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
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
