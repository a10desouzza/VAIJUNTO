// Arquivo: internal/cliente/tcp.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Abre uma conexão TCP com a empresa e envia uma requisição JSON terminada por linha.
// Recebe e interpreta a resposta, aplicando prazos e limites de tamanho.
// As telas reutilizam essa função sem precisar implementar o transporte de rede.

package cliente

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"vaijunto/internal/protocolo"
)

// EnviarRequisicao Serializa a requisicao, conecta com prazo de 5 segundos e define 30 segundos
// para a operacao. Envia JSON com quebra de linha, le a resposta limitada a 16 MiB e verifica seu
// status. Fecha o socket ao sair. Erro de recepcao nao prova que o servidor deixou de executar o
// pedido.
// Recebe: endereco string, req protocolo.Requisicao.
// Retorna: a resposta decodificada ou erro de transporte/protocolo. Erro de transporte nao garante que o servidor deixou de executar o pedido.
func EnviarRequisicao(endereco string, req protocolo.Requisicao) (*protocolo.Resposta, error) {
	dados, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if len(dados) >= protocolo.LimiteMensagem {
		return nil, fmt.Errorf("requisição excede 1 MiB")
	}
	/* Conectar tem prazo de 5 s; leitura e escrita da operacao recebem prazo de 30 s. */
	conn, err := net.DialTimeout("tcp", endereco, 5*time.Second)
	if err != nil {
		return nil, err
	}
	// Fechar a conexao ao sair da funcao, mesmo que haja erro.
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return nil, err
	}
	writer := bufio.NewWriter(conn)
	if _, err := writer.Write(append(dados, '\n')); err != nil {
		return nil, err
	}
	/* Flush envia os bytes que ainda estao no buffer do cliente. */
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
