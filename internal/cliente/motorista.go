// internal/cliente/motorista.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Formularios de publicacao, consulta e cancelamento de caronas.

package cliente

import (
	"fmt"
	"strings"
	"time"
	"vaijunto/internal/protocolo"
)

// publicar: monta rota e trechos, mostra o resumo e envia apos a confirmacao do motorista.
// Retorna: nil ao publicar ou desistir na confirmacao; error de entrada, geracao da chave ou
// envio.
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

// caronas: consulta e mostra vagas e passageiros por trecho. Retorna a lista para selecao.
// Retorna: Lista de caronas exibidas e nil, ou nil e error se a consulta falhar.
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

// cancelarCarona: seleciona uma oferta e pede confirmacao antes de solicitar o cancelamento.
// Retorna: nil ao cancelar ou voltar; error de consulta ou cancelamento.
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
