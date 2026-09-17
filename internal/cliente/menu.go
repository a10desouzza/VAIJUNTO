// internal/cliente/menu.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Interface dos dois perfis. Le formularios, monta requisicoes e apresenta respostas.
// As regras finais de disponibilidade e permissao ficam no servidor.

package cliente

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"vaijunto/internal/protocolo"
)

type menu struct {
	leitor   *bufio.Scanner
	saida    io.Writer
	endereco string
	perfil   string
	sessaoID string
	err      error

	nome string
}

// ExecutarMenu: mantem o fluxo de login e operacoes. Entrada e saida sao parametros para permitir
// testes. Retorna: nil ao sair normalmente ou atingir EOF; error de leitura quando nao houver
// encerramento normal.
func ExecutarMenu(endereco, perfil string, entrada io.Reader, saida io.Writer) error {
	m := &menu{leitor: bufio.NewScanner(entrada), saida: saida, endereco: endereco, perfil: perfil}
	m.cabecalho("VaiJunto", "")
	fmt.Fprintf(saida, "  Perfil: %s  |  Servidor: %s\n", perfil, endereco)
	fmt.Fprintln(saida, "  Digite /voltar em qualquer formulário para retornar ao menu.")
	defer func() {
		if m.sessaoID != "" {
			EnviarRequisicao(endereco, protocolo.Requisicao{Acao: protocolo.AcaoSair, SessaoID: m.sessaoID, Dados: json.RawMessage(`{}`)})
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
			if perfil == protocolo.Passageiro {
				if err := m.notificacoes(false); err != nil {
					fmt.Fprintln(saida, "Não foi possível verificar notificações:", err)
				}
				if m.sessaoID == "" {
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
	fmt.Fprintln(saida, "\nCliente encerrado.")
	if errors.Is(m.err, io.EOF) {
		return nil
	}
	return m.err
}

// executarOpcao: recebe a opcao escolhida e usa o perfil guardado no menu para chamar a operacao.
// Retorna: o erro da operacao ou nil no sucesso. Trocar de conta limpa a sessao apos o logout.
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
