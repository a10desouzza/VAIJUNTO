/* ================================================================================================
 * internal/cliente/entrada.go - VaiJunto: sistema de caronas compartilhadas
 * Autor: Arthur Souza
 *
 * Leitura e apresentacao dos campos. Repete valores invalidos e formata datas e dinheiro.
 *
 * DIVISAO DE RESPONSABILIDADES:
 * O cliente coleta entradas e exibe respostas; o servidor decide permissoes, disponibilidade e
 * alteracoes nas reservas.
 * ================================================================================================ */

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

/* cabecalho
 *
 * Recebe: titulo: nome da tela; detalhe: texto adicional opcional; m: destino de saida.
 *
 * O que faz: imprime titulo e detalhe opcional. Sem retorno.
 *
 * Retorna: Nao retorna valor. Escreve o cabecalho no terminal ou no buffer do teste.
 */
func (m *menu) cabecalho(titulo, detalhe string) {
	fmt.Fprintf(m.saida, "\n--- %s ---\n", titulo)
	if detalhe != "" {
		fmt.Fprintln(m.saida, detalhe)
	}
}

/* opcoes
 *
 * Recebe: opcoes: quantidade variavel de textos; m: destino de saida.
 *
 * O que faz: imprime as opcoes recebidas. Sem retorno.
 *
 * Retorna: Nao retorna valor. Imprime uma opcao por linha.
 */
func (m *menu) opcoes(opcoes ...string) {
	for _, opcao := range opcoes {
		fmt.Fprintln(m.saida, opcao)
	}
	fmt.Fprintln(m.saida)
}

/* campo
 *
 * Recebe: rotulo: nome do campo; normalizar: funcao opcional de ajuste; validar: funcao que aceita
 * ou recusa;
 * m: estado de entrada e saida.
 *
 * O que faz: normaliza o texto quando solicitado e repete a entrada enquanto a validacao indicar
 * erro.
 *
 * Retorna: Texto validado ou string vazia na interrupcao, que fica registrada em m.err.
 */
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

/* email
 *
 * Recebe: Nenhum argumento explicito; usa m para ler e validar o e-mail.
 *
 * O que faz: le o e-mail e converte para minusculas antes de validar.
 *
 * Retorna: E-mail em minusculas ou string vazia se o formulario foi interrompido.
 */
func (m *menu) email() string {
	return m.campo("E-mail", strings.ToLower, protocolo.ValidarEmail)
}

/* cidade
 *
 * Recebe: rotulo: nome do campo; anteriores: cidades que nao podem ser repetidas; m: estado do
 * formulario.
 *
 * O que faz: le uma cidade e impede repeticao entre os nomes ja informados.
 *
 * Retorna: Cidade validada com espacos normalizados ou string vazia na interrupcao.
 */
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

/* dinheiro
 *
 * Recebe: valor: quantia em reais.
 *
 * O que faz: formata reais com duas casas decimais e virgula.
 *
 * Retorna: Texto com R$, virgula e duas casas decimais, sem alterar o valor original.
 */
func dinheiro(valor float64) string {
	return "R$ " + strings.ReplaceAll(fmt.Sprintf("%.2f", valor), ".", ",")
}

/* horarioLegivel
 *
 * Recebe: valor: data e hora em RFC3339.
 *
 * O que faz: converte RFC3339 para data, hora e fuso; preserva o texto se a conversao falhar.
 *
 * Retorna: Texto com data, hora e fuso; se nao conseguir interpretar, retorna o texto original.
 */
func horarioLegivel(valor string) string {
	t, err := time.Parse(time.RFC3339, valor)
	if err != nil {
		return valor
	}
	return t.Format("02/01/2006 às 15:04 (-07:00)")
}

/* caracteresSeguros
 *
 * Recebe: s: texto recebido do terminal.
 *
 * O que faz: rejeita controles e caracteres de formatacao invisiveis na entrada do terminal.
 *
 * Retorna: false se encontrar controle ou caractere invisivel de formatacao; true caso contrario.
 */
func caracteresSeguros(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return true
}
