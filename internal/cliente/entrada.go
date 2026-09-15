package cliente

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"vaijunto/internal/protocolo"
)

var voltar = errors.New("voltar ao menu")

func (m *menu) cabecalho(titulo, detalhe string) {
	fmt.Fprintf(m.saida, "\n--- %s ---\n", titulo)
	if detalhe != "" {
		fmt.Fprintln(m.saida, detalhe)
	}
}

func (m *menu) opcoes(opcoes ...string) {
	for _, opcao := range opcoes {
		fmt.Fprintln(m.saida, opcao)
	}
	fmt.Fprintln(m.saida)
}
func (m *menu) campo(rotulo string, normalizar func(string) string, validar func(string) error) string {
	for m.err == nil {
		valor := m.texto(rotulo)
		if m.err != nil {
			return ""
		}
		if normalizar != nil {
			valor = normalizar(valor)
		}
		if err := validar(valor); err != nil {
			fmt.Fprintln(m.saida, err.Error())
			continue
		}
		return valor
	}
	return ""
}

func (m *menu) email() string {
	return m.campo("E-mail", strings.ToLower, protocolo.ValidarEmail)
}

func (m *menu) cidade(rotulo string, anteriores []string) string {
	return m.campo(rotulo, func(s string) string { return strings.Join(strings.Fields(s), " ") }, func(s string) error {
		if err := protocolo.ValidarCidade(s); err != nil {
			return err
		}
		for _, anterior := range anteriores {
			if strings.EqualFold(s, anterior) {
				return fmt.Errorf("esta cidade já foi informada; escolha uma cidade diferente")
			}
		}
		return nil
	})
}

func dinheiro(valor float64) string {
	return "R$ " + strings.ReplaceAll(fmt.Sprintf("%.2f", valor), ".", ",")
}

func horarioLegivel(valor string) string {
	t, err := time.Parse(time.RFC3339, valor)
	if err != nil {
		return valor
	}
	return t.Format("02/01/2006 às 15:04 (-07:00)")
}

func caracteresSeguros(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return true
}
