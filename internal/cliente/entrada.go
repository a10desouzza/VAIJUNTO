// internal/cliente/entrada.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Leitura e apresentacao dos campos. Repete valores invalidos e formata datas e dinheiro.

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

// cria uma variavel global chamada voltar
var voltar = errors.New("voltar ao menu")

// cabecalho: imprime titulo e detalhe opcional. Sem retorno.
func (m *menu) cabecalho(titulo, detalhe string) {
	fmt.Fprintf(m.saida, "\n--- %s ---\n", titulo)
	if detalhe != "" { // imprime detalhe se houver
		fmt.Fprintln(m.saida, detalhe)
	}
}

// opcoes: imprime as opcoes recebidas. Sem retorno.
func (m *menu) opcoes(opcoes ...string) {
	// imprime cada opcao em uma linha
	for _, opcao := range opcoes {
		fmt.Fprintln(m.saida, opcao)
	}
	fmt.Fprintln(m.saida)
}

// campo: normaliza o texto quando solicitado e repete a entrada enquanto a validacao indicar erro.
// Retorna: Texto validado ou string vazia na interrupcao, que fica registrada em m.err.
func (m *menu) campo(rotulo string, normalizar func(string) string, validar func(string) error) string {
	for m.err == nil { // repete enquanto nao houver interrupcao
		valor := m.texto(rotulo) // pede ao usuario um texto

		if m.err != nil { // se usuario saiu do formulario ou entrada acabou, retorna string vazia
			return ""
		}
		if normalizar != nil { // se houver funcao de normalizacao, aplica ao valor
			valor = normalizar(valor)
		}
		// tenta validar o valor; se houver erro, imprime e repete a leitura
		if err := validar(valor); err != nil {
			fmt.Fprintln(m.saida, err.Error())
			continue
		}
		return valor
	}
	return ""
}

// email: le o e-mail e converte para minusculas antes de validar. Retorna: E-mail em minusculas ou
// string vazia se o formulario foi interrompido.
func (m *menu) email() string {
	return m.campo("E-mail", strings.ToLower, protocolo.ValidarEmail)
}

// cidade: le uma cidade e impede repeticao entre os nomes ja informados. Retorna: Cidade validada
// com espacos normalizados ou string vazia na interrupcao.
func (m *menu) cidade(rotulo string, anteriores []string) string {
	// normaliza espacos e valida cidade; repete se houver erro ou repeticao
	return m.campo(rotulo, func(s string) string { return strings.Join(strings.Fields(s), " ") }, func(s string) error {
		//usa a validacao padrao da cidade, se falhar devolve erro
		if err := protocolo.ValidarCidade(s); err != nil {
			return err
		}
		// percorre cidades ja informadas
		for _, anterior := range anteriores {
			// compara o texto ignorando maiusculas e minusculas
			if strings.EqualFold(s, anterior) {
				// impede rota com cidades repetidas
				return fmt.Errorf("esta cidade já foi informada; escolha uma cidade diferente")
			}
		}
		return nil // se chegou aqui, a cidade e valida e nao repetida
	})
}

// dinheiro: formata reais com duas casas decimais e virgula. Retorna: Texto com R$, virgula e duas
// casas decimais, sem alterar o valor original.
func dinheiro(valor float64) string {
	return "R$ " + strings.ReplaceAll(fmt.Sprintf("%.2f", valor), ".", ",")
}

// horarioLegivel: converte RFC3339 para data, hora e fuso; preserva o texto se a conversao falhar.
// Retorna: Texto com data, hora e fuso; se nao conseguir interpretar, retorna o texto original.
func horarioLegivel(valor string) string {
	t, err := time.Parse(time.RFC3339, valor) // tenta interpretar horario como RFC3339
	if err != nil {
		return valor
	}
	return t.Format("02/01/2006 às 15:04 (-07:00)") // formata horario pra uma pessoa ler
}

// caracteresSeguros: rejeita controles e caracteres de formatacao invisiveis na entrada do
// terminal. Retorna: false se encontrar controle ou caractere invisivel de formatacao; true caso
// contrario.
func caracteresSeguros(s string) bool {
	//percorre cada caractere da string
	for _, r := range s {
		// se for caractere de controle ou invisivel de formatacao, retorna falso
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return true // se chegou aqui, todos os caracteres sao seguros
}

// cria uma expressao para validar precos
var formatoDecimal = regexp.MustCompile(`^[0-9]+(\.[0-9]{1,2})?$`)

// texto: le uma linha preenchida. Registra EOF ou /voltar em m.err para interromper o formulario.
// Retorna: Texto preenchido e sem espacos nas pontas; string vazia se houver interrupcao,
// registrada em m.err.
func (m *menu) texto(rotulo string) string {
	for m.err == nil {
		fmt.Fprintf(m.saida, "%s: ", rotulo) // mostra a pergunta
		if !m.leitor.Scan() {
			m.err = m.leitor.Err()
			if m.err == nil {
				m.err = io.EOF
			}
			return ""
		}
		s := strings.TrimSpace(m.leitor.Text())
		// se o usuario digitou /voltar, interrompe o formulario e retorna string vazia
		if s == "/voltar" {
			m.err = voltar
			return ""
		}
		// se houver caracteres de controle ou invisiveis, avisa e repete a leitura
		if !caracteresSeguros(s) {
			fmt.Fprintln(m.saida, "Não use caracteres de controle neste campo.")
			continue
		}
		// se nao esta vazio, retorna o texto; senao, avisa e repete a leitura
		if s != "" {
			return s
		}
		fmt.Fprintln(m.saida, "Preencha este campo.") // se o usuario so apertou enter, pede pra preencher e repete a leitura
	}
	return ""
}

// numero: repete a leitura ate receber um inteiro no intervalo ou interromper o formulario.
// Retorna: Inteiro validado; zero na interrupcao. Consulte m.err para distinguir zero valido de
// interrupcao.
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

// preco: atalho para leitura de um valor em reais usando a validacao decimal. Retorna: Valor em
// reais entre zero e um milhao; zero na interrupcao, indicada por m.err.
func (m *menu) preco() float64 {
	return m.decimal("Preço em reais (ex.: 25,50)", 0, 1000000)
}

// decimal: aceita ponto ou virgula e ate duas casas decimais dentro dos limites. Retorna: float64
// validado ou zero quando interrompido; a interrupcao fica em m.err.
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

// data: aceita data brasileira ou ISO e retorna AAAA-MM-DD para o protocolo. Retorna: Data
// normalizada em AAAA-MM-DD; string vazia quando interrompida, com m.err preenchido.
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
