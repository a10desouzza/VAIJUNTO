// internal/cliente/sessao.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Cadastro, login e envio de operacoes com a sessao do usuario.

package cliente

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"vaijunto/internal/protocolo"
)

// enviar: Monta o envelope uma vez e preserva os dados nas tentativas. Em falha de transporte
// permite ao usuario reenviar o mesmo pedido, preservando a chave de publicacao/reserva. Invalida
// a sessao local ao receber erro de sessao expirada e converte os dados para alvo. Retorna: nil no
// sucesso, preenchendo alvo se fornecido; error de formulario, transporte ou recusa do servidor.
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

// cadastrar: le os dados, confirma a senha, cria a conta e guarda a sessao devolvida pelo
// servidor. Retorna: nil apos cadastrar e entrar automaticamente, ou error de formulario, rede ou
// recusa.
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

// entrar: autentica e guarda a sessao localmente. Recusa uma conta de perfil diferente do cliente
// aberto. Retorna: nil com sessao e nome armazenados em m; error se o login falhar ou o perfil for
// diferente.
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

// novaChave: gera 16 bytes aleatorios em hexadecimal para identificar uma nova publicacao ou
// confirmacao. Retorna: String hexadecimal com 16 bytes aleatorios e nil; string vazia e error se
// a geracao falhar.
func novaChave() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
