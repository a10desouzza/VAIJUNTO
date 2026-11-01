// Arquivo: internal/cliente/menu.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Lê a configuração, permite escolher a empresa e controla a navegação do cliente.
// Faz cadastro e login, mantém a sessão e encaminha as opções de cada perfil.
// Quando uma chamada é repetida, preserva os dados e a chave da operação.

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
	"os"
	"strings"

	"vaijunto/internal/configuracao"
	"vaijunto/internal/protocolo"
)

// Executar recebe o perfil, interpreta o endereco do servidor e abre o menu interativo.
// Recebe: perfil string.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
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
// Recebe: variavel, padrao string.
// Retorna: string.
func enderecoDaEmpresa(variavel, padrao string) string {
	if endereco := os.Getenv(variavel); endereco != "" {
		return endereco
	}
	return padrao
}

type menu struct {
	leitor   *bufio.Scanner // ler entrada do usuario
	saida    io.Writer      // escrever saida para o usuario
	endereco string         // endereco do servidor
	perfil   string         // perfil do usuario (motorista ou passageiro)
	sessaoID string         // id da sessao do usuario apos login
	err      error          // erro de leitura ou interrupcao do usuario

	nome    string // nome do usuario logado, para apresentar no menu
	empresa string
}

// ExecutarMenu mantem o fluxo de login e operacoes. Entrada e saida sao parametros para permitir
// testes.
// Recebe: endereco, perfil string, entrada io.Reader, saida io.Writer.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
func ExecutarMenu(endereco, perfil string, entrada io.Reader, saida io.Writer) error {
	m := &menu{leitor: bufio.NewScanner(entrada), saida: saida, endereco: endereco, perfil: perfil}
	return m.executar()
}

// ExecutarComSelecao usa o mesmo leitor durante a escolha da empresa e o login.
// Assim nenhuma linha ja lida pelo buffer se perde ao mudar de tela.
// Recebe: empresas []configuracao.ServidorCliente, perfil string, entrada io.Reader, saida io.Writer.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
func ExecutarComSelecao(empresas []configuracao.ServidorCliente, perfil string, entrada io.Reader, saida io.Writer) error {
	if len(empresas) == 0 {
		return fmt.Errorf("nenhuma empresa configurada")
	}
	m := &menu{leitor: bufio.NewScanner(entrada), saida: saida, perfil: perfil}
	for {
		m.cabecalho("Escolha uma empresa", "")
		for i, empresa := range empresas {
			fmt.Fprintf(saida, "%d - %s\n", i+1, empresa.Nome)
		}
		fmt.Fprintln(saida, "0 - Sair")
		escolha := m.numero("Empresa", 0, len(empresas))
		if errors.Is(m.err, voltar) {
			m.err = nil
			continue
		}
		if errors.Is(m.err, io.EOF) {
			fmt.Fprintln(saida, "Cliente encerrado.")
			return nil
		}
		if m.err != nil {
			return m.err
		}
		if escolha == 0 {
			fmt.Fprintln(saida, "Cliente encerrado.")
			return nil
		}
		selecionada := empresas[escolha-1]
		if selecionada.Endereco == "" {
			return fmt.Errorf("endereco da empresa %s nao configurado", selecionada.Nome)
		}
		m.endereco, m.empresa = selecionada.Endereco, selecionada.Nome
		return m.executar()
	}
}

// executar mantem a sessao e as operacoes na empresa escolhida ate sair do cliente.
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: nenhum parametro.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
func (m *menu) executar() error {
	if err := configuracao.ValidarEnderecoCliente(m.endereco); err != nil {
		return err
	}
	m.cabecalho("VaiJunto", "")
	if m.empresa != "" {
		fmt.Fprintf(m.saida, "  Empresa: %s\n", m.empresa)
	}
	fmt.Fprintf(m.saida, "  Perfil: %s  |  Servidor: %s\n", m.perfil, m.endereco)
	fmt.Fprintln(m.saida, "  Digite /voltar em qualquer formulário para retornar ao menu.")
	// garante que o logout seja enviado ao servidor quando o menu for encerrado
	defer func() {
		if m.sessaoID != "" {
			EnviarRequisicao(m.endereco, protocolo.Requisicao{Acao: protocolo.AcaoSair, SessaoID: m.sessaoID, Dados: json.RawMessage(`{}`)})
		}
	}()
	for m.err == nil {
		var err error
		if m.sessaoID == "" {
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
			// Se for passageiro, o menu tenta buscar notificações antes de mostrar as opções
			if m.perfil == protocolo.Passageiro {
				if err := m.notificacoes(false); err != nil {
					fmt.Fprintln(m.saida, "Não foi possível verificar notificações:", err)
				}
				if m.sessaoID == "" {
					continue
				}
			}
			if m.perfil == protocolo.Motorista {
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
			err = m.executarOpcao(opcao)
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
	fmt.Fprintln(m.saida, "\nCliente encerrado.")
	if errors.Is(m.err, io.EOF) {
		return nil
	}
	return m.err
}

// executarOpcao recebe a opcao escolhida e usa o perfil guardado no menu para chamar a operacao.
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: opcao int.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
func (m *menu) executarOpcao(opcao int) error {
	if opcao == 4 {
		err := m.enviar(protocolo.AcaoSair, struct{}{}, nil)
		if err == nil {
			m.sessaoID = ""
		}
		return err
	}
	if m.perfil == protocolo.Motorista {
		switch opcao {
		case 1:
			return m.publicar()
		case 2:
			_, err := m.caronas()
			return err
		case 3:
			return m.cancelarCarona()
		case 5:
			return m.cancelarTrecho()
		}
	} else {
		switch opcao {
		case 1:
			return m.buscar()
		case 2:
			_, err := m.reservas()
			return err
		case 3:
			return m.cancelarReserva()
		case 5:
			return m.notificacoes(true)
		}
	}
	return nil
}

// enviar Monta o envelope uma vez e preserva os dados nas tentativas. Em falha de transporte
// permite ao usuario reenviar o mesmo pedido, preservando a chave de publicacao/reserva. Invalida
// a sessao local ao receber erro de sessao expirada e converte os dados para alvo.
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: acao string, dados, alvo any.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
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
	req := protocolo.Requisicao{Acao: acao, SessaoID: m.sessaoID, Dados: raw}
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
				m.sessaoID = ""
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

// cadastrar le os dados, confirma a senha, cria a conta e guarda a sessao devolvida pelo
// servidor.
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: nenhum parametro.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
func (m *menu) cadastrar() error {
	m.cabecalho("Cadastro", "")
	p := protocolo.Cadastro{Nome: m.campo("Nome", func(s string) string {
		return strings.Join(strings.Fields(s), " ")
	}, protocolo.ValidarNome), Email: m.email(), Perfil: m.perfil}
	for m.err == nil {
		p.Senha = m.campo("Senha (8 a 128; visível no terminal)", nil, protocolo.ValidarSenha)
		confirmacao := m.texto("Repita a senha")
		if m.err != nil || p.Senha == confirmacao {
			break
		}
		fmt.Fprintln(m.saida, "As senhas não coincidem. Digite novamente.")
	}
	var sessao protocolo.Sessao
	if err := m.enviar(protocolo.AcaoCadastrar, p, &sessao); err != nil {
		return err
	}
	if sessao.Usuario.Perfil != m.perfil || sessao.ID == "" {
		return fmt.Errorf("sessão de cadastro inválida")
	}
	m.sessaoID = sessao.ID
	m.nome = sessao.Usuario.Nome
	fmt.Fprintf(m.saida, "\nConta criada. Acesso realizado: %s\n", sessao.Usuario.Nome)
	return nil
}

// entrar autentica e guarda a sessao localmente. Recusa uma conta de perfil diferente do cliente
// aberto.
// Usa o receptor m para acessar entrada, saida e estado do menu.
// Recebe: nenhum parametro.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
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

// novaChave gera 16 bytes aleatorios em hexadecimal para identificar uma nova publicacao ou
// confirmacao.
// Recebe: nenhum parametro.
// Retorna: (string, error). Em falha, devolve error; em sucesso, erro nil.
func novaChave() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
