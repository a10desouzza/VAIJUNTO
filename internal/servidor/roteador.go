/* ================================================================================================
 * internal/servidor/roteador.go - VaiJunto: sistema de caronas compartilhadas
 * Autor: Arthur Souza
 *
 * Interpretacao do protocolo. Valida o envelope e converte os dados conforme a acao solicitada.
 *
 * DIVISAO DE RESPONSABILIDADES:
 * O servidor mantem o estado em RAM e valida as operacoes. As estruturas compartilhadas sao
 * protegidas por travas.
 * ================================================================================================ */

package servidor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"
	"vaijunto/internal/protocolo"
)

/* verificarJSON
 *
 * Recebe: d: decodificador posicionado no valor JSON que sera percorrido.
 *
 * O que faz: percorre objetos e listas para rejeitar chaves duplicadas, inclusive nos dados
 * internos.
 *
 * Retorna: nil quando nao encontra erro; error de leitura ou campo duplicado.
 */
func verificarJSON(d *json.Decoder) error {
	elemento, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := elemento.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		vistos := make(map[string]bool)
		for d.More() {
			chave, err := d.Token()
			if err != nil {
				return err
			}
			nome, ok := chave.(string)
			if !ok || vistos[nome] {
				return fmt.Errorf("campo duplicado ou inválido")
			}
			vistos[nome] = true
			if err := verificarJSON(d); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := verificarJSON(d); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("JSON inválido")
	}
	_, err = d.Token()
	return err
}

/* decodificar
 *
 * Recebe: dados: bytes do objeto JSON; alvo: ponteiro para a estrutura que recebera os campos.
 *
 * O que faz: exige um unico objeto JSON UTF-8, rejeita campos desconhecidos e preenche alvo ou
 * retorna erro.
 *
 * Retorna: nil com alvo preenchido, ou error de formato, UTF-8, duplicidade ou campos
 * desconhecidos.
 */
func decodificar(dados []byte, alvo any) error {
	dados = bytes.TrimSpace(dados)
	if len(dados) == 0 || dados[0] != '{' || !utf8.Valid(dados) || !json.Valid(dados) {
		return fmt.Errorf("objeto JSON UTF-8 obrigatório")
	}
	if err := verificarJSON(json.NewDecoder(bytes.NewReader(dados))); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(dados))
	d.DisallowUnknownFields()
	if err := d.Decode(alvo); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("esperado um único objeto JSON")
	}
	return nil
}

/* ProcessarMensagem
 *
 * Recebe: mensagem: bytes de uma linha sem o delimitador final; g: estado central.
 *
 * O que faz: transforma o resultado da operacao em uma resposta padrao de SUCESSO ou ERRO.
 *
 * Retorna: Bytes da resposta JSON terminada por quebra de linha, com status SUCESSO ou ERRO.
 */
func ProcessarMensagem(mensagem []byte, g *GrafoItinerarios) []byte {
	dados, err := processar(mensagem, g)
	if err != nil {
		return codificarResposta(protocolo.Resposta{Status: protocolo.Erro, Mensagem: err.Error()})
	}
	return codificarResposta(protocolo.Resposta{Status: protocolo.Sucesso, Mensagem: "Operação concluída.", Dados: dados})
}

/* processar
 *
 * Recebe: mensagem: envelope JSON; g: gerenciador usado para executar a acao.
 *
 * O que faz: le o envelope e chama a funcao da acao. Cada operacao possui seu proprio tipo de
 * dados.
 *
 * Retorna: Dados da operacao como any e nil no sucesso, ou error que sera convertido em resposta.
 */
func processar(mensagem []byte, g *GrafoItinerarios) (any, error) {
	if len(mensagem) >= protocolo.LimiteMensagem {
		return nil, fmt.Errorf("mensagem excede o limite de 1 MiB")
	}
	var req protocolo.Requisicao
	if err := decodificar(mensagem, &req); err != nil {
		return nil, fmt.Errorf("envelope inválido: %w", err)
	}
	switch req.Acao {
	case protocolo.AcaoCadastrar:
		var p protocolo.Cadastro
		if err := decodificar(req.Dados, &p); err != nil {
			return nil, err
		}
		if _, err := g.Cadastrar(p); err != nil {
			return nil, err
		}
		return g.Autenticar(protocolo.Credenciais{Email: p.Email, Senha: p.Senha})
	case protocolo.AcaoEntrar:
		var p protocolo.Credenciais
		if err := decodificar(req.Dados, &p); err != nil {
			return nil, err
		}
		return g.Autenticar(p)
	case protocolo.AcaoPublicar:
		var p protocolo.PublicacaoCarona
		if err := decodificar(req.Dados, &p); err != nil {
			return nil, err
		}
		return g.Publicar(req.SessaoID, p)
	case protocolo.AcaoBuscar:
		var p protocolo.BuscaItinerario
		if err := decodificar(req.Dados, &p); err != nil {
			return nil, err
		}
		return g.Buscar(req.SessaoID, p)
	case protocolo.AcaoConfirmar:
		var p protocolo.ReservaItinerario
		if err := decodificar(req.Dados, &p); err != nil {
			return nil, err
		}
		return g.Confirmar(req.SessaoID, p)
	case protocolo.AcaoCancelarCarona, protocolo.AcaoCancelarReserva, protocolo.AcaoCancelarTrecho, protocolo.AcaoLerNotificacao:
		var p protocolo.Identificador
		if err := decodificar(req.Dados, &p); err != nil {
			return nil, err
		}
		if req.Acao == protocolo.AcaoCancelarCarona {
			return g.CancelarCarona(req.SessaoID, p.ID)
		}
		if req.Acao == protocolo.AcaoCancelarTrecho {
			return g.CancelarTrecho(req.SessaoID, p.ID)
		}
		if req.Acao == protocolo.AcaoLerNotificacao {
			return nil, g.LerNotificacao(req.SessaoID, p.ID)
		}
		return g.CancelarReserva(req.SessaoID, p.ID)
	case protocolo.AcaoCaronas, protocolo.AcaoReservas, protocolo.AcaoSair, protocolo.AcaoNotificacoes:
		if err := decodificar(req.Dados, &struct{}{}); err != nil {
			return nil, err
		}
		if req.Acao == protocolo.AcaoNotificacoes {
			return g.ConsultarNotificacoes(req.SessaoID)
		}
		if req.Acao == protocolo.AcaoCaronas {
			return g.ConsultarCaronas(req.SessaoID)
		}
		if req.Acao == protocolo.AcaoReservas {
			return g.ConsultarReservas(req.SessaoID)
		}
		return nil, g.Desconectar(req.SessaoID)
	default:
		return nil, fmt.Errorf("ação desconhecida")
	}
}

/* codificarResposta
 *
 * Recebe: r: envelope de resposta com status, mensagem e dados opcionais.
 *
 * O que faz: serializa a resposta e acrescenta a quebra de linha exigida pelo protocolo.
 *
 * Retorna: Linha JSON em bytes; se a serializacao falhar, devolve uma resposta fixa de erro.
 */
func codificarResposta(r protocolo.Resposta) []byte {
	dados, err := json.Marshal(r)
	if err != nil {
		return []byte("{\"status\":\"ERRO\",\"mensagem\":\"Falha ao codificar resposta.\"}\n")
	}
	return append(dados, '\n')
}
