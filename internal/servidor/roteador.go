package servidor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"
	"vaijunto/internal/protocolo"
)

func verificarJSON(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
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

func ProcessarMensagem(mensagem []byte, g *GrafoItinerarios) []byte {
	dados, err := processar(mensagem, g)
	if err != nil {
		return codificarResposta(protocolo.Resposta{Status: protocolo.Erro, Mensagem: err.Error()})
	}
	return codificarResposta(protocolo.Resposta{Status: protocolo.Sucesso, Mensagem: "Operação concluída.", Dados: dados})
}

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
		return g.Cadastrar(p)
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
		return g.Publicar(req.Token, p)
	case protocolo.AcaoBuscar:
		var p protocolo.BuscaItinerario
		if err := decodificar(req.Dados, &p); err != nil {
			return nil, err
		}
		return g.Buscar(req.Token, p)
	case protocolo.AcaoConfirmar:
		var p protocolo.ReservaItinerario
		if err := decodificar(req.Dados, &p); err != nil {
			return nil, err
		}
		return g.Confirmar(req.Token, p)
	case protocolo.AcaoCancelarCarona, protocolo.AcaoCancelarReserva, protocolo.AcaoCancelarTrecho, protocolo.AcaoLerNotificacao:
		var p protocolo.Identificador
		if err := decodificar(req.Dados, &p); err != nil {
			return nil, err
		}
		if req.Acao == protocolo.AcaoCancelarCarona {
			return g.CancelarCarona(req.Token, p.ID)
		}
		if req.Acao == protocolo.AcaoCancelarTrecho {
			return g.CancelarTrecho(req.Token, p.ID)
		}
		if req.Acao == protocolo.AcaoLerNotificacao {
			return nil, g.LerNotificacao(req.Token, p.ID)
		}
		return g.CancelarReserva(req.Token, p.ID)
	case protocolo.AcaoCaronas, protocolo.AcaoReservas, protocolo.AcaoSair, protocolo.AcaoNotificacoes:
		if err := decodificar(req.Dados, &struct{}{}); err != nil {
			return nil, err
		}
		if req.Acao == protocolo.AcaoNotificacoes {
			return g.ConsultarNotificacoes(req.Token)
		}
		if req.Acao == protocolo.AcaoCaronas {
			return g.ConsultarCaronas(req.Token)
		}
		if req.Acao == protocolo.AcaoReservas {
			return g.ConsultarReservas(req.Token)
		}
		return nil, g.Desconectar(req.Token)
	default:
		return nil, fmt.Errorf("ação desconhecida")
	}
}

func codificarResposta(r protocolo.Resposta) []byte {
	dados, err := json.Marshal(r)
	if err != nil {
		return []byte("{\"status\":\"ERRO\",\"mensagem\":\"Falha ao codificar resposta.\"}\n")
	}
	return append(dados, '\n')
}
