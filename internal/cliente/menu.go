// Autor: Arthur de Andrade Souza.
// Matéria: TEC502 - TP01 - Concorrência e Conectividade - 2026.2.
// Declaração de não plágio: Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
// Funcionalidade: Controla a escolha da empresa, a sessão, os menus e o reenvio de solicitações do cliente.
package cliente

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"vaijunto/internal/configuracao"
	"vaijunto/internal/protocolo"
)

type menu struct {
	leitor   *bufio.Scanner
	saida    io.Writer
	endereco string
	perfil   string
	sessaoID string
	err      error
	nome     string
	empresa  string
}

// ExecutarMenu mantem o fluxo de login e operacoes. Entrada e saida sao parametros para permitir testes.
// Recebe: endereço, perfil e canais de entrada/saída.
func ExecutarMenu(endereco, perfil string, entrada io.Reader, saida io.Writer) error {
	m := &menu{leitor: bufio.NewScanner(entrada), saida: saida, endereco: endereco, perfil: perfil}
	return m.executar()
}

// ExecutarComSelecao usa o mesmo leitor durante a escolha da empresa e o login. Assim nenhuma linha ja lida pelo buffer se perde ao mudar de tela.
// Recebe: empresas disponíveis, perfil e entrada/saída.
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
// Recebe: estado do menu m, sem argumentos explícitos.
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
// Recebe: opção escolhida e perfil guardado em m.
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

// enviar Monta o envelope uma vez e preserva os dados nas tentativas. Em falha de transporte permite ao usuario reenviar o mesmo pedido, preservando a chave de publicacao/reserva. Invalida a sessao local ao receber erro de sessao expirada e converte os dados para alvo.
// Recebe: ação, dados e ponteiro alvo, usando a sessão de m.
func (m *menu) enviar(acao string, dados, alvo any) error {

	if m.err != nil {
		return m.err
	}
	raw, err := json.Marshal(dados)
	if err != nil {
		return err
	}

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

// cadastrar le os dados, confirma a senha, cria a conta e guarda a sessao devolvida pelo servidor.
// Recebe: estado e entrada do menu m.
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

// notificacoes Consulta os avisos do usuario, mostra cada aviso selecionado e so depois envia a confirmacao de leitura dos novos. Se essa confirmacao falhar, o aviso pode aparecer novamente.
// Recebe: indicação de mostrar todos os avisos e menu m.
func (m *menu) notificacoes(todas bool) error {
	resp, err := EnviarRequisicao(m.endereco, protocolo.Requisicao{Acao: protocolo.AcaoNotificacoes, SessaoID: m.sessaoID, Dados: json.RawMessage(`{}`)})
	if err != nil {
		return err
	}

	if resp.Status != protocolo.Sucesso {
		m.sessaoID = ""
		return fmt.Errorf("%s", resp.Mensagem)
	}
	dados, err := json.Marshal(resp.Dados)
	if err != nil {
		return err
	}
	var avisos []protocolo.Notificacao
	if err := json.Unmarshal(dados, &avisos); err != nil {
		return err
	}
	if todas && len(avisos) == 0 {
		fmt.Fprintln(m.saida, "Nenhuma notificação.")
	}
	for _, aviso := range avisos {
		if aviso.Lida && !todas {
			continue
		}
		fmt.Fprintf(m.saida, "\nAviso da reserva %s (%s):\n%s\n", aviso.ReservaID, horarioLegivel(aviso.CriadaEm), aviso.Mensagem)

		if !aviso.Lida {
			raw, _ := json.Marshal(protocolo.Identificador{ID: aviso.ID})
			confirmacao, err := EnviarRequisicao(m.endereco, protocolo.Requisicao{Acao: protocolo.AcaoLerNotificacao, SessaoID: m.sessaoID, Dados: raw})
			if err != nil {
				return err
			}
			if confirmacao.Status != protocolo.Sucesso {
				return fmt.Errorf("%s", confirmacao.Mensagem)
			}
		}
	}
	return nil
}
