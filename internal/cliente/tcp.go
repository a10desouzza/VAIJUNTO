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

func EnviarRequisicao(endereco string, req protocolo.Requisicao) (*protocolo.Resposta, error) {
	dados, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if len(dados) >= protocolo.LimiteMensagem {
		return nil, fmt.Errorf("requisição excede 1 MiB")
	}
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
