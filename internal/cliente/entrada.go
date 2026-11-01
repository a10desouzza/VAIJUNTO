// Arquivo: internal/cliente/entrada.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Implementa a leitura dos campos e a apresentação de informações no terminal.
// Repete entradas inválidas e trata datas, valores, cidades e números dentro dos limites.
// Registra fim da entrada ou /voltar para que o formulário possa ser interrompido.

package cliente

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"vaijunto/internal/protocolo"
)

// voltar encerra o formulario atual sem enviar uma requisicao.
var voltar = errors.New("voltar ao menu")

// cabecalho imprime titulo e detalhe opcional. Sem retorno.
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: titulo, detalhe string.
// Retorna: nenhum valor; executa os efeitos descritos acima.
func (m *menu) cabecalho(titulo, detalhe string) {
	fmt.Fprintf(m.saida, "\n--- %s ---\n", titulo)
	if detalhe != "" {
		fmt.Fprintln(m.saida, detalhe)
	}
}

// opcoes imprime as opcoes recebidas. Sem retorno.
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: opcoes ...string.
// Retorna: nenhum valor; executa os efeitos descritos acima.
func (m *menu) opcoes(opcoes ...string) {
	for _, opcao := range opcoes {
		fmt.Fprintln(m.saida, opcao)
	}
	fmt.Fprintln(m.saida)
}

// campo normaliza o texto quando solicitado e repete a entrada enquanto a validacao indicar erro.
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: rotulo string, normalizar func(string) string, validar func(string) error.
// Retorna: o texto normalizado e validado, ou texto vazio se a entrada for interrompida; o motivo fica em m.err.
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

// email le o e-mail e converte para minusculas antes de validar.
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: nenhum parametro.
// Retorna: string.
func (m *menu) email() string {
	return m.campo("E-mail", strings.ToLower, protocolo.ValidarEmail)
}

// cidade le uma cidade e impede repeticao entre os nomes ja informados.
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: rotulo string, anteriores []string.
// Retorna: string.
func (m *menu) cidade(rotulo string, anteriores []string) string {
	return m.campo(rotulo, func(s string) string {
		return strings.Join(strings.Fields(s), " ")
	}, func(s string) error {
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

// dinheiro formata reais com duas casas decimais e virgula.
// Recebe: valor float64.
// Retorna: string.
func dinheiro(valor float64) string {
	return "R$ " + strings.ReplaceAll(fmt.Sprintf("%.2f", valor), ".", ",")
}

// horarioLegivel converte RFC3339 para data, hora e fuso; preserva o texto se a conversao falhar.
// Recebe: valor string.
// Retorna: string.
func horarioLegivel(valor string) string {
	t, err := time.Parse(time.RFC3339, valor)
	if err != nil {
		return valor
	}
	return t.Format("02/01/2006 às 15:04 (-07:00)")
}

// caracteresSeguros rejeita controles e caracteres de formatacao invisiveis na entrada do
// Recebe: s string.
// Retorna: true quando o texto nao contem controles nem caracteres Unicode de formatacao; false caso contrario.
func caracteresSeguros(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return true
}

// O campo decimal aceita ponto ou virgula, com ate duas casas.
var formatoDecimal = regexp.MustCompile(`^[0-9]+(\.[0-9]{1,2})?$`)

// texto le uma linha preenchida. Registra EOF ou /voltar em m.err para interromper o formulario.
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: rotulo string.
// Retorna: string.
func (m *menu) texto(rotulo string) string {
	for m.err == nil {
		fmt.Fprintf(m.saida, "%s: ", rotulo)
		if !m.leitor.Scan() {
			m.err = m.leitor.Err()
			if m.err == nil {
				m.err = io.EOF
			}
			return ""
		}
		s := strings.TrimSpace(m.leitor.Text())
		if s == "/voltar" {
			m.err = voltar
			return ""
		}
		if !caracteresSeguros(s) {
			fmt.Fprintln(m.saida, "Não use caracteres de controle neste campo.")
			continue
		}
		if s != "" {
			return s
		}
		fmt.Fprintln(m.saida, "Preencha este campo.")
	}
	return ""
}

// numero repete a leitura ate receber um inteiro no intervalo ou interromper o formulario.
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: rotulo string, minimo, maximo int.
// Retorna: o inteiro aceito no intervalo ou zero em interrupcao; consultar m.err para distinguir interrupcao de um zero valido.
func (m *menu) numero(rotulo string, minimo, maximo int) int {
	for m.err == nil {
		s := m.texto(rotulo)
		if m.err != nil {
			return 0
		}
		n, err := strconv.Atoi(s)
		if err == nil && n >= minimo && n <= maximo {
			return n
		}
		fmt.Fprintf(m.saida, "Digite um número inteiro entre %d e %d.\n", minimo, maximo)
	}
	return 0
}

// preco atalho para leitura de um valor em reais usando a validacao decimal.
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: nenhum parametro.
// Retorna: float64.
func (m *menu) preco() float64 {
	return m.decimal("Preço em reais (ex.: 25,50)", 0, 1000000)
}

// decimal aceita ponto ou virgula e ate duas casas decimais dentro dos limites.
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: rotulo string, minimo, maximo float64.
// Retorna: o decimal aceito nos limites ou zero em interrupcao; o motivo fica em m.err.
func (m *menu) decimal(rotulo string, minimo, maximo float64) float64 {
	for m.err == nil {
		s := strings.ReplaceAll(m.texto(rotulo), ",", ".")
		if m.err != nil {
			return 0
		}
		p, err := strconv.ParseFloat(s, 64)
		if err == nil && formatoDecimal.MatchString(s) && p >= minimo && p <= maximo {
			return p
		}
		fmt.Fprintf(m.saida, "Informe um valor de %.2f a %.2f com até duas casas decimais.\n", minimo, maximo)
	}
	return 0
}

// data aceita data brasileira ou ISO e retorna AAAA-MM-DD para o protocolo.
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: nenhum parametro.
// Retorna: a data valida em AAAA-MM-DD ou texto vazio em interrupcao; o motivo fica em m.err.
func (m *menu) data() string {
	for m.err == nil {
		s := m.texto("Data (DD/MM/AAAA ou AAAA-MM-DD)")
		if m.err != nil {
			return ""
		}
		for _, formato := range []string{"2006-01-02", "02/01/2006"} {
			if data, err := time.Parse(formato, s); err == nil {
				return data.Format("2006-01-02")
			}
		}
		fmt.Fprintln(m.saida, "Data inexistente ou inválida. Exemplo: 01/10/2026.")
	}
	return ""
}
