/* ================================================================================================
 * internal/cliente/tcp.go - VaiJunto: sistema de caronas compartilhadas
 * Autor: Arthur Souza
 *
 * Comunicacao reutilizavel: abre um socket por operacao, envia NDJSON e fecha apos a resposta.
 * O token identifica a sessao mesmo quando a conexao anterior ja foi encerrada.
 *
 * DIVISAO DE RESPONSABILIDADES:
 * O cliente coleta entradas e exibe respostas; o servidor decide permissoes, disponibilidade e
 * alteracoes nas reservas.
 * ================================================================================================ */

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

/* EnviarRequisicao
 *
 * Recebe: endereco: IP ou nome e porta do servidor; req: envelope com acao, token e dados.
 *
 * O que faz: Serializa a requisicao, conecta com prazo de 5 segundos e define 30 segundos para a
 * operacao.
 * Envia JSON com quebra de linha, le a resposta limitada a 16 MiB e verifica seu status.
 * Fecha o socket ao sair. Erro de recepcao nao prova que o servidor deixou de executar o pedido.
 *
 * Retorna: Ponteiro para Resposta e nil quando recebida e interpretada; nil e error na falha.
 * Uma resposta com status ERRO ainda e uma resposta valida, que deve ser tratada pelo chamador.
 */
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
