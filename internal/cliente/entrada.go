// Autor: Arthur de Andrade Souza.
// Matéria: TEC502 - TP01 - Concorrência e Conectividade - 2026.2.
// Declaração de não plágio: Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
// Funcionalidade: Lê e valida entradas do terminal, configura destinos e troca mensagens NDJSON por socket TCP.
package cliente

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"vaijunto/internal/configuracao"
	"vaijunto/internal/protocolo"
)

var voltar = errors.New("voltar ao menu")

// cabecalho imprime titulo e detalhe opcional. Sem retorno.
// Recebe: título, detalhe opcional e saída de m.
func (m *menu) cabecalho(titulo, detalhe string) {
	fmt.Fprintf(m.saida, "\n--- %s ---\n", titulo)
	if detalhe != "" {
		fmt.Fprintln(m.saida, detalhe)
	}
}

// opcoes imprime as opcoes recebidas. Sem retorno.
// Recebe: lista variável de opções e saída de m.
func (m *menu) opcoes(opcoes ...string) {
	for _, opcao := range opcoes {
		fmt.Fprintln(m.saida, opcao)
	}
	fmt.Fprintln(m.saida)
}

// campo normaliza o texto quando solicitado e repete a entrada enquanto a validacao indicar erro.
// Recebe: rótulo, normalizador opcional e validador, no menu m.
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
// Recebe: entrada do menu m, sem argumentos explícitos.
func (m *menu) email() string {
	return m.campo("E-mail", strings.ToLower, protocolo.ValidarEmail)
}

// cidade le uma cidade e impede repeticao entre os nomes ja informados.
// Recebe: rótulo e cidades anteriores, no menu m.
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
// Recebe: valor em reais.
func dinheiro(valor float64) string {
	return "R$ " + strings.ReplaceAll(fmt.Sprintf("%.2f", valor), ".", ",")
}

// horarioLegivel converte RFC3339 para data, hora e fuso; preserva o texto se a conversao falhar.
// Recebe: instante RFC3339 em texto.
func horarioLegivel(valor string) string {
	t, err := time.Parse(time.RFC3339, valor)
	if err != nil {
		return valor
	}
	return t.Format("02/01/2006 às 15:04 (-07:00)")
}

// caracteresSeguros rejeita controles e caracteres de formatacao invisiveis na entrada do
// Recebe: texto digitado.
func caracteresSeguros(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return true
}

var formatoDecimal = regexp.MustCompile(`^[0-9]+(\.[0-9]{1,2})?$`)

// texto le uma linha preenchida. Registra EOF ou /voltar em m.err para interromper o formulario.
// Recebe: rótulo do campo e entrada do menu m.
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
// Recebe: rótulo e limites mínimo/máximo, no menu m.
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
// Recebe: entrada do menu m, sem argumentos explícitos.
func (m *menu) preco() float64 {
	return m.decimal("Preço em reais (ex.: 25,50)", 0, 1000000)
}

// decimal aceita ponto ou virgula e ate duas casas decimais dentro dos limites.
// Recebe: rótulo e limites, no menu m.
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
// Recebe: entrada do menu m, sem argumentos explícitos.
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

// Executar recebe o perfil, interpreta o endereco do servidor e abre o menu interativo.
// Recebe: perfil do cliente; lê opções e ambiente.
func Executar(perfil string) error {
	endereco := flag.String("servidor", os.Getenv("VAIJUNTO_SERVIDOR"), "Endereco TCP direto; quando informado, dispensa a escolha da empresa")
	vaijunto := flag.String("vaijunto", enderecoDaEmpresa("VAIJUNTO_SERVIDOR_VAIJUNTO", "localhost:7001"), "Endereco TCP do VAIJUNTO")
	minas := flag.String("minas", enderecoDaEmpresa("VAIJUNTO_SERVIDOR_MINAS", "localhost:7002"), "Endereco TCP da Minas")
	saoPaulo := flag.String("sao-paulo", enderecoDaEmpresa("VAIJUNTO_SERVIDOR_SAO_PAULO", "localhost:7003"), "Endereco TCP de Sao Paulo")
	flag.Parse()
	if *endereco != "" {
		return ExecutarMenu(*endereco, perfil, os.Stdin, os.Stdout)
	}
	empresas := []configuracao.ServidorCliente{
		{Nome: "VAIJUNTO", Endereco: *vaijunto},
		{Nome: "Minas", Endereco: *minas},
		{Nome: "São Paulo", Endereco: *saoPaulo},
	}
	return ExecutarComSelecao(empresas, perfil, os.Stdin, os.Stdout)
}

// enderecoDaEmpresa obtem o endereco TCP da variavel de ambiente ou usa o padrao recebido.
// Recebe: nome da variável de ambiente e endereço padrão.
func enderecoDaEmpresa(variavel, padrao string) string {
	if endereco := os.Getenv(variavel); endereco != "" {
		return endereco
	}
	return padrao
}

// novaChave gera 16 bytes aleatorios em hexadecimal para identificar uma nova publicacao ou confirmacao.
// Recebe: nenhum argumento.
func novaChave() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// EnviarRequisicao Serializa a requisicao, conecta com prazo de 5 segundos e define 30 segundos para a operacao. Envia JSON com quebra de linha, le a resposta limitada a 16 MiB e verifica seu status. Fecha o socket ao sair. Erro de recepcao nao prova que o servidor deixou de executar o pedido.
// Recebe: endereço TCP e requisição do protocolo.
func EnviarRequisicao(endereco string, req protocolo.Requisicao) (*protocolo.Resposta, error) {
	dados, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if len(dados) >= protocolo.LimiteMensagem {
		return nil, fmt.Errorf("requisição excede 1 MiB")
	}

	conn, err := net.DialTimeout("tcp", endereco, 5*time.Second)
	if err != nil {
		return nil, err
	}

	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return nil, err
	}
	writer := bufio.NewWriter(conn)
	if _, err := writer.Write(append(dados, '\n')); err != nil {
		return nil, err
	}

	if err := writer.Flush(); err != nil {
		return nil, err
	}
	resposta, err := bufio.NewReader(io.LimitReader(conn, 16*protocolo.LimiteMensagem+1)).ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("resposta NDJSON incompleta: %w", err)
	}
	if len(resposta) > 16*protocolo.LimiteMensagem {
		return nil, fmt.Errorf("resposta excede 16 MiB")
	}
	var resp protocolo.Resposta
	if err := json.Unmarshal(resposta, &resp); err != nil {
		return nil, err
	}
	if resp.Status != protocolo.Sucesso && resp.Status != protocolo.Erro {
		return nil, fmt.Errorf("status inválido")
	}
	return &resp, nil
}
