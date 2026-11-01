// Arquivo: internal/servidor/roteador.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Interpreta o envelope JSON recebido e identifica a ação solicitada pelo cliente.
// Rejeita mensagens inválidas e converte os dados para o tipo esperado pela operação.
// Encaminha a chamada, salva alterações e produz a resposta padronizada de sucesso ou erro.

package servidor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"vaijunto/internal/protocolo"
)

// verificarJSON percorre objetos e listas para rejeitar chaves duplicadas, inclusive nos dados
// internos.
// Recebe: d *json.Decoder.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
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

// decodificar exige um unico objeto JSON UTF-8, rejeita campos desconhecidos e preenche alvo ou
// retorna erro.
// Recebe: dados []byte, alvo any.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
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
	// json.Valid acima ja exige um unico valor JSON completo.
	return d.Decode(alvo)
}

// ProcessarMensagem transforma o resultado da operacao em uma resposta padrao de SUCESSO ou ERRO.
// Recebe: mensagem []byte, g *GrafoItinerarios.
// Retorna: a resposta JSON padronizada em bytes, terminada por quebra de linha, inclusive para operacoes recusadas.
func ProcessarMensagem(mensagem []byte, g *GrafoItinerarios) []byte {
	dados, err := processar(mensagem, g)
	if !mensagemDeConsulta(mensagem) {
		if backupErr := g.SalvarBackup(); backupErr != nil {
			g.logger.Printf("BACKUP erro=%v", backupErr)
			err = backupErr
		}
	}
	if err != nil {
		return codificarResposta(protocolo.Resposta{Status: protocolo.Erro, Mensagem: err.Error()})
	}
	return codificarResposta(protocolo.Resposta{Status: protocolo.Sucesso, Mensagem: "Operação concluída.", Dados: dados})
}

// Consultas nao gravam o arquivo. Uma mensagem desconhecida segue pela fila de
// escrita para nunca classificar uma alteracao como leitura por engano.
// Recebe: mensagem []byte.
// Retorna: bool.
func mensagemDeConsulta(mensagem []byte) bool {
	var req protocolo.Requisicao
	if json.Unmarshal(mensagem, &req) != nil {
		return false
	}
	switch req.Acao {
	case protocolo.AcaoBuscar, protocolo.AcaoCaronas, protocolo.AcaoReservas, protocolo.AcaoNotificacoes:
		return true
	default:
		return false
	}
}

// processar le o envelope e chama a funcao da acao. Cada operacao possui seu proprio tipo de
// dados.
// Recebe: mensagem []byte, g *GrafoItinerarios.
// Retorna: (any, error). Em falha, devolve error; em sucesso, erro nil.
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
		if g.distribuido != nil {
			return g.distribuido.buscar(req.SessaoID, p)
		}
		return g.Buscar(req.SessaoID, p)
	case protocolo.AcaoConfirmar:
		var p protocolo.ReservaItinerario
		if err := decodificar(req.Dados, &p); err != nil {
			return nil, err
		}
		if g.distribuido != nil {
			return g.distribuido.reservar(req.SessaoID, p)
		}
		return g.Confirmar(req.SessaoID, p)
	case protocolo.AcaoCancelarCarona, protocolo.AcaoCancelarReserva, protocolo.AcaoCancelarTrecho, protocolo.AcaoLerNotificacao:
		var p protocolo.Identificador
		if err := decodificar(req.Dados, &p); err != nil {
			return nil, err
		}
		if req.Acao == protocolo.AcaoCancelarCarona {
			if g.distribuido != nil {
				return g.distribuido.cancelarOferta(req.SessaoID, p.ID, false)
			}
			return g.CancelarCarona(req.SessaoID, p.ID)
		}
		if req.Acao == protocolo.AcaoCancelarTrecho {
			if g.distribuido != nil {
				return g.distribuido.cancelarOferta(req.SessaoID, p.ID, true)
			}
			return g.CancelarTrecho(req.SessaoID, p.ID)
		}
		if req.Acao == protocolo.AcaoLerNotificacao {
			return nil, g.LerNotificacao(req.SessaoID, p.ID)
		}
		if g.distribuido != nil {
			return g.distribuido.cancelar(req.SessaoID, p.ID)
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
			if g.distribuido != nil {
				return g.distribuido.consultar(req.SessaoID)
			}
			return g.ConsultarReservas(req.SessaoID)
		}
		return nil, g.Desconectar(req.SessaoID)
	default:
		return nil, fmt.Errorf("ação desconhecida")
	}
}

// codificarResposta serializa a resposta e acrescenta a quebra de linha exigida pelo protocolo.
// Recebe: r protocolo.Resposta.
// Retorna: []byte.
func codificarResposta(r protocolo.Resposta) []byte {
	dados, err := json.Marshal(r)
	if err != nil {
		return []byte("{\"status\":\"ERRO\",\"mensagem\":\"Falha ao codificar resposta.\"}\n")
	}
	return append(dados, '\n')
}
