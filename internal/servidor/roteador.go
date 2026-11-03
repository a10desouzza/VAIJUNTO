// TEC502 - Problema 2. Autor: Arthur de Andrade Souza.
// Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
package servidor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
	"vaijunto/internal/configuracao"
	"vaijunto/internal/protocolo"
)

// verificarJSON percorre objetos e listas para rejeitar chaves duplicadas, inclusive nos dados
// internos.
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
func codificarResposta(r protocolo.Resposta) []byte {
	dados, err := json.Marshal(r)
	if err != nil {
		return []byte("{\"status\":\"ERRO\",\"mensagem\":\"Falha ao codificar resposta.\"}\n")
	}
	return append(dados, '\n')
}

// validarPreparacao valida identidade, participantes, horario e lista de trechos do pedido de preparacao.
func validarPreparacao(pedido protocolo.PedidoPreparacao) error {
	if strings.TrimSpace(pedido.TransacaoID) == "" || len(pedido.TransacaoID) > 128 {
		return fmt.Errorf("transacao invalida")
	}

	if err := configuracao.ValidarEmpresaID(pedido.EmpresaOrigem); err != nil {
		return err
	}

	if strings.TrimSpace(pedido.PassageiroRef) == "" || len(pedido.PassageiroRef) > 128 {
		return fmt.Errorf("referencia de passageiro invalida")
	}
	if pedido.InicioItinerario != "" {
		if _, err := time.Parse(time.RFC3339, pedido.InicioItinerario); err != nil {
			return fmt.Errorf("início do itinerário inválido")
		}
	}

	if len(pedido.Participantes) == 0 {
		return fmt.Errorf("nenhum participante informado")
	}

	participantes := make(map[string]bool)

	for _, empresa := range pedido.Participantes {
		if err := configuracao.ValidarEmpresaID(empresa); err != nil {
			return err
		}

		if participantes[empresa] {
			return fmt.Errorf("empresa %s informada mais de uma vez", empresa)
		}
		participantes[empresa] = true
	}

	if len(pedido.TrechosIDs) < 1 || len(pedido.TrechosIDs) > 20 {
		return fmt.Errorf("informe de 1 a 20 trechos locais")
	}
	trechos := make(map[string]bool)

	for _, id := range pedido.TrechosIDs {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("trecho invalido")
		}

		if trechos[id] {
			return fmt.Errorf("trecho %s informado mais de uma vez", id)
		}

		trechos[id] = true
	}

	return nil
}
