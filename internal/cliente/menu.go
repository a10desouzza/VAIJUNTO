/* ================================================================================================
 * internal/cliente/menu.go - VaiJunto: sistema de caronas compartilhadas
 * Autor: Arthur Souza
 *
 * Interface dos dois perfis. Le formularios, monta requisicoes e apresenta respostas.
 * As regras finais de disponibilidade e permissao ficam no servidor.
 *
 * DIVISAO DE RESPONSABILIDADES:
 * O cliente coleta entradas e exibe respostas; o servidor decide permissoes, disponibilidade e
 * alteracoes nas reservas.
 * ================================================================================================ */

package cliente

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"vaijunto/internal/protocolo"
)

type menu struct {
	leitor   *bufio.Scanner
	saida    io.Writer
	endereco string
	perfil   string
	token    string
	err      error

	nome string
}

/* ExecutarMenu
 *
 * Recebe: endereco: servidor TCP; perfil: papel do cliente; entrada: leitor das respostas
 * digitadas;
 * saida: destino dos textos apresentados. Cria m para guardar a sessao e o estado do menu.
 *
 * O que faz: mantem o fluxo de login e operacoes. Entrada e saida sao parametros para permitir
 * testes.
 *
 * Retorna: nil ao sair normalmente ou atingir EOF; error de leitura quando nao houver encerramento
 * normal.
 */
func ExecutarMenu(endereco, perfil string, entrada io.Reader, saida io.Writer) error {
	m := &menu{leitor: bufio.NewScanner(entrada), saida: saida, endereco: endereco, perfil: perfil}
	m.cabecalho("VaiJunto", "")
	fmt.Fprintf(saida, "  Perfil: %s  |  Servidor: %s\n", perfil, endereco)
	fmt.Fprintln(saida, "  Digite /voltar em qualquer formulário para retornar ao menu.")
	defer func() {
		if m.token != "" {
			EnviarRequisicao(endereco, protocolo.Requisicao{Acao: protocolo.AcaoSair, Token: m.token, Dados: json.RawMessage(`{}`)})
		}
	}()
	for m.err == nil {
		var err error
		if m.token == "" {
			m.cabecalho("Menu inicial", "")
			m.opcoes("1 - Entrar", "2 - Criar conta", "0 - Sair")
			opcao := m.numero("Opção", 0, 2)
			if errors.Is(m.err, voltar) {
				m.err = nil
				continue
			}
			if m.err != nil || opcao == 0 {
				break
			}
			if opcao == 1 {
				err = m.entrar()
			} else {
				err = m.cadastrar()
			}
		} else {
			if perfil == protocolo.Passageiro {
				if err := m.notificacoes(false); err != nil {
					fmt.Fprintln(saida, "Não foi possível verificar notificações:", err)
				}
				if m.token == "" {
					continue
				}
			}
			if perfil == protocolo.Motorista {
				m.cabecalho("Motorista", m.nome)
				m.opcoes("1 - Publicar carona", "2 - Minhas caronas e passageiros", "3 - Cancelar carona", "4 - Trocar de conta", "5 - Cancelar trecho", "0 - Sair")
			} else {
				m.cabecalho("Passageiro", m.nome)
				m.opcoes("1 - Buscar e reservar itinerário", "2 - Minhas reservas", "3 - Cancelar reserva", "4 - Trocar de conta", "5 - Verificar notificações", "0 - Sair")
			}
			opcao := m.numero("Opção", 0, 5)
			if errors.Is(m.err, voltar) {
				m.err = nil
				continue
			}
			if m.err != nil || opcao == 0 {
				break
			}
			switch opcao {
			case 5:
				if perfil == protocolo.Motorista {
					err = m.cancelarTrecho()
				} else {
					err = m.notificacoes(true)
				}
			case 1:
				if perfil == protocolo.Motorista {
					err = m.publicar()
				} else {
					err = m.buscar()
				}
			case 2:
				if perfil == protocolo.Motorista {
					_, err = m.caronas()
				} else {
					_, err = m.reservas()
				}
			case 3:
				if perfil == protocolo.Motorista {
					err = m.cancelarCarona()
				} else {
					err = m.cancelarReserva()
				}
			case 4:
				err = m.enviar(protocolo.AcaoSair, struct{}{}, nil)
				if err == nil {
					m.token = ""
				}
			}
		}
		if errors.Is(m.err, voltar) {
			m.err = nil
			fmt.Fprintln(m.saida, "Operação interrompida. Você voltou ao menu.")
			continue
		}
		if err != nil && !errors.Is(err, io.EOF) {
			fmt.Fprintln(m.saida, err.Error())
		}
	}
	fmt.Fprintln(saida, "\nCliente encerrado.")
	if errors.Is(m.err, io.EOF) {
		return nil
	}
	return m.err
}

/* texto
 *
 * Recebe: rotulo: nome do campo; m: leitor, saida e estado do formulario.
 *
 * O que faz: le uma linha preenchida. Registra EOF ou /voltar em m.err para interromper o
 * formulario.
 *
 * Retorna: Texto preenchido e sem espacos nas pontas; string vazia se houver interrupcao,
 * registrada em m.err.
 */
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

/* numero
 *
 * Recebe: rotulo: nome do campo; minimo e maximo: intervalo permitido; m: estado do formulario.
 *
 * O que faz: repete a leitura ate receber um inteiro no intervalo ou interromper o formulario.
 *
 * Retorna: Inteiro validado; zero na interrupcao. Consulte m.err para distinguir zero valido de
 * interrupcao.
 */
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

/* preco
 *
 * Recebe: Nenhum argumento explicito; usa m para entrada e saida.
 *
 * O que faz: atalho para leitura de um valor em reais usando a validacao decimal.
 *
 * Retorna: Valor em reais entre zero e um milhao; zero na interrupcao, indicada por m.err.
 */
func (m *menu) preco() float64 {
	return m.decimal("Preço em reais (ex.: 25,50)", 0, 1000000)
}

/* decimal
 *
 * Recebe: rotulo: nome do campo; minimo e maximo: limites numericos; m: estado do formulario.
 *
 * O que faz: aceita ponto ou virgula e ate duas casas decimais dentro dos limites.
 *
 * Retorna: float64 validado ou zero quando interrompido; a interrupcao fica em m.err.
 */
func (m *menu) decimal(rotulo string, minimo, maximo float64) float64 {
	for m.err == nil {
		s := strings.ReplaceAll(m.texto(rotulo), ",", ".")
		if m.err != nil {
			return 0
		}
		p, err := strconv.ParseFloat(s, 64)
		if err == nil && regexp.MustCompile(`^[0-9]+(\.[0-9]{1,2})?$`).MatchString(s) && !math.IsNaN(p) && !math.IsInf(p, 0) && p >= minimo && p <= maximo && math.Abs(p*100-math.Round(p*100)) < 0.000001 {
			return p
		}
		fmt.Fprintf(m.saida, "Informe um valor de %.2f a %.2f com até duas casas decimais.\n", minimo, maximo)
	}
	return 0
}

/* data
 *
 * Recebe: Nenhum argumento explicito; usa m para ler a data.
 *
 * O que faz: aceita data brasileira ou ISO e retorna AAAA-MM-DD para o protocolo.
 *
 * Retorna: Data normalizada em AAAA-MM-DD; string vazia quando interrompida, com m.err preenchido.
 */
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

/* enviar
 *
 * Recebe: acao: operacao solicitada; dados: objeto a serializar; alvo: ponteiro para receber os
 * dados da resposta,
 * ou nil quando nao precisa deles; m: endereco, token e estado do formulario.
 *
 * O que faz: Monta o envelope uma vez e preserva os dados nas tentativas. Em falha de transporte
 * permite
 * ao usuario reenviar o mesmo pedido, preservando a chave de publicacao/reserva.
 * Invalida o token local ao receber erro de sessao expirada e converte os dados para alvo.
 *
 * Retorna: nil no sucesso, preenchendo alvo se fornecido; error de formulario, transporte ou
 * recusa do servidor.
 */
func (m *menu) enviar(acao string, dados, alvo any) error {
	/* /voltar e EOF interrompem antes de qualquer envio deste formulario. */
	if m.err != nil {
		return m.err
	}
	raw, err := json.Marshal(dados)
	if err != nil {
		return err
	}
	/* Montamos uma vez, fora das tentativas: o reenvio preserva dados e chave da operacao. */
	req := protocolo.Requisicao{Acao: acao, Token: m.token, Dados: raw}
	for {
		resp, err := EnviarRequisicao(m.endereco, req)
		if err != nil {
			fmt.Fprintf(m.saida, "\nNão foi possível obter a resposta: %v\n", err)
			fmt.Fprintln(m.saida, "1. Tentar novamente a mesma operação\n0. Voltar (consulte o resultado antes de repetir a operação)")
			if m.numero("Opção", 0, 1) != 1 || m.err != nil {
				return fmt.Errorf("resposta não recebida; o servidor pode ter concluído a operação")
			}
			continue
		}
		if resp.Status != protocolo.Sucesso {
			if strings.Contains(resp.Mensagem, "sessão inválida ou expirada") {
				m.token = ""
			}
			return errors.New(resp.Mensagem)
		}
		if alvo == nil {
			return nil
		}
		raw, err = json.Marshal(resp.Dados)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, alvo)
	}
}

/* cadastrar
 *
 * Recebe: Nenhum argumento explicito; usa m para ler nome, e-mail, senha e perfil.
 *
 * O que faz: le os dados, confirma a senha e envia o cadastro com o perfil deste cliente.
 *
 * Retorna: nil apos cadastrar ou error de formulario, rede ou recusa. O login e feito
 * separadamente.
 */
func (m *menu) cadastrar() error {
	m.cabecalho("Cadastro", "")
	p := protocolo.Cadastro{Nome: m.campo("Nome", func(s string) string { return strings.Join(strings.Fields(s), " ") }, protocolo.ValidarNome), Email: m.email(), Perfil: m.perfil}
	for m.err == nil {
		p.Senha = m.campo("Senha (8 a 128; visível no terminal)", nil, protocolo.ValidarSenha)
		confirmacao := m.texto("Repita a senha")
		if m.err != nil || p.Senha == confirmacao {
			break
		}
		fmt.Fprintln(m.saida, "As senhas não coincidem. Digite novamente.")
	}
	if err := m.enviar(protocolo.AcaoCadastrar, p, nil); err != nil {
		return err
	}
	fmt.Fprintln(m.saida, "Conta criada. Escolha Entrar para acessar.")
	return nil
}

/* entrar
 *
 * Recebe: Nenhum argumento explicito; usa m para ler credenciais e guardar a sessao.
 *
 * O que faz: autentica e guarda o token localmente. Recusa uma conta de perfil diferente do
 * cliente aberto.
 *
 * Retorna: nil com token e nome armazenados em m; error se o login falhar ou o perfil for
 * diferente.
 */
func (m *menu) entrar() error {
	m.cabecalho("Login", "")
	p := protocolo.Credenciais{Email: m.email(), Senha: m.campo("Senha (visível no terminal)", nil, protocolo.ValidarSenha)}
	var sessao protocolo.Sessao
	if err := m.enviar(protocolo.AcaoEntrar, p, &sessao); err != nil {
		return err
	}
	if sessao.Usuario.Perfil != m.perfil {
		EnviarRequisicao(m.endereco, protocolo.Requisicao{Acao: protocolo.AcaoSair, Token: sessao.Token, Dados: json.RawMessage(`{}`)})
		return fmt.Errorf("esta conta pertence ao perfil %s; abra o cliente correspondente", sessao.Usuario.Perfil)
	}
	m.token = sessao.Token
	m.nome = sessao.Usuario.Nome
	fmt.Fprintf(m.saida, "\nLogin realizado: %s\n", sessao.Usuario.Nome)
	return nil
}

/* novaChave
 *
 * Recebe: Nao recebe parametros.
 *
 * O que faz: gera 16 bytes aleatorios em hexadecimal para identificar uma nova publicacao ou
 * confirmacao.
 *
 * Retorna: String hexadecimal com 16 bytes aleatorios e nil; string vazia e error se a geracao
 * falhar.
 */
func novaChave() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

/* publicar
 *
 * Recebe: Nenhum argumento explicito; usa m para ler rota, partida, vagas, valor/km, distancias e
 * tempos.
 *
 * O que faz: monta rota e trechos, mostra o resumo e envia apos a confirmacao do motorista.
 *
 * Retorna: nil ao publicar ou desistir na confirmacao; error de entrada, geracao da chave ou envio.
 */
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
	p.ValorKM = m.decimal("Valor por quilômetro em reais (por passageiro)", 0, 1000000)
	for i := 0; i < n-1 && m.err == nil; i++ {
		fmt.Fprintf(m.saida, "\nTrecho %d: %s -> %s\n", i+1, p.Rota[i], p.Rota[i+1])
		distancia := m.decimal("Distância deste trecho em km", 0.01, 100000)
		for m.err == nil && distancia*p.ValorKM > 1000000 {
			fmt.Fprintln(m.saida, "O preço do trecho não pode ultrapassar R$ 1000000.")
			distancia = m.decimal("Distância deste trecho em km", 0.01, 100000)
		}
		p.Trechos = append(p.Trechos, protocolo.OfertaTrecho{DistanciaKM: distancia, TempoViagem: m.numero("Tempo de viagem em minutos", 1, 10080), TempoParada: m.numero("Parada após este trecho em minutos", 0, 10080)})
	}
	if m.err != nil {
		return m.err
	}
	m.cabecalho("Confirmar carona", strings.Join(p.Rota, " -> "))
	fmt.Fprintf(m.saida, "  Partida: %s\n  Assentos: %d\n", horarioLegivel(p.DataHora), p.Assentos)
	for i, t := range p.Trechos {
		centavos := (int64(math.Round(p.ValorKM*100))*int64(math.Round(t.DistanciaKM*100)) + 50) / 100
		fmt.Fprintf(m.saida, "  %s -> %s: %.2f km x %s/km = %s | %d min + %d min de parada\n", p.Rota[i], p.Rota[i+1], t.DistanciaKM, dinheiro(p.ValorKM), dinheiro(float64(centavos)/100), t.TempoViagem, t.TempoParada)
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

/* caronas
 *
 * Recebe: Nenhum argumento explicito; usa m.token e m.endereco.
 *
 * O que faz: consulta e mostra vagas e passageiros por trecho. Retorna a lista para selecao.
 *
 * Retorna: Lista de caronas exibidas e nil, ou nil e error se a consulta falhar.
 */
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

/* cancelarCarona
 *
 * Recebe: Nenhum argumento explicito; usa m para consultar e selecionar a carona.
 *
 * O que faz: seleciona uma oferta e pede confirmacao antes de solicitar o cancelamento.
 *
 * Retorna: nil ao cancelar ou voltar; error de consulta ou cancelamento.
 */
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

/* buscar
 *
 * Recebe: Nenhum argumento explicito; usa m para receber filtros e selecionar um itinerario.
 *
 * O que faz: consulta itinerarios e permite reservar uma opcao enviando os IDs na ordem do
 * percurso.
 *
 * Retorna: nil ao reservar, voltar ou nao encontrar opcoes; error de entrada, chave ou requisicao.
 */
func (m *menu) buscar() error {
	m.cabecalho("Buscar itinerário", "")
	b := protocolo.BuscaItinerario{Origem: m.cidade("Origem", nil)}
	b.Destino = m.cidade("Destino", []string{b.Origem})
	b.Data = m.data()
	fmt.Fprintln(m.saida, "Ordenar por: 1. Menor preço | 2. Menor duração | 3. Menos trechos")
	ordem := m.numero("Opção", 1, 3)
	if m.err != nil {
		return m.err
	}
	b.OrdenarPor = []string{"PRECO", "TEMPO", "TRECHOS"}[ordem-1]
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

/* mostrarReserva
 *
 * Recebe: r: reserva que sera exibida; m: destino de saida.
 *
 * O que faz: mostra status, valor e percurso. Nao apresenta uma escolha de assento numerado.
 *
 * Retorna: Nao retorna valor. Mostra ID, status, preco, percurso e motivo de cancelamento quando
 * existir.
 */
func (m *menu) mostrarReserva(r protocolo.Reserva) {
	fmt.Fprintf(m.saida, "  %s | %s | %s\n", r.ID, r.Status, dinheiro(r.PrecoTotal))
	for _, a := range r.Assentos {
		fmt.Fprintf(m.saida, "  %s -> %s\n    %s | 1 vaga reservada\n", a.Trecho.Origem, a.Trecho.Destino, horarioLegivel(a.Trecho.DataHora))
	}
	if r.Motivo != "" {
		fmt.Fprintln(m.saida, "  ", r.Motivo)
	}
}

/* reservas
 *
 * Recebe: Nenhum argumento explicito; usa m.token e m.endereco.
 *
 * O que faz: consulta e apresenta as reservas do passageiro. Retorna a lista para as outras
 * operacoes.
 *
 * Retorna: Lista de reservas exibidas e nil; nil e error se a consulta falhar.
 */
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

/* cancelarReserva
 *
 * Recebe: Nenhum argumento explicito; usa m para consultar e selecionar uma reserva.
 *
 * O que faz: seleciona a reserva e confirma o cancelamento de todos os seus trechos.
 *
 * Retorna: nil ao cancelar ou voltar; error de consulta ou cancelamento.
 */
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
