// Autor: Arthur de Andrade Souza.
// Matéria: TEC502 - TP01 - Concorrência e Conectividade - 2026.2.
// Declaração de não plágio: Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
// Funcionalidade: Transporta mensagens REST do 2PC, propaga Lamport e persiste o estado antes de responder com sucesso.
package servidor

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"vaijunto/internal/protocolo"
)

const cabecalhoLamport = "X-Lamport"

// responderCoordenacao marca o envio e persiste antes de emitir a resposta.
// Recebe: escritor HTTP, resultado e possível erro.
func (d *Distribuido) responderCoordenacao(w http.ResponseWriter, v any, err error) {
	marca := d.evento(0)
	if backupErr := d.g.SalvarBackup(); backupErr != nil {
		err = backupErr
	}
	w.Header().Set(cabecalhoLamport, strconv.FormatUint(marca, 10))
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// Handler autentica e recebe as chamadas REST de coordenacao.
// Recebe: estado da API ou do componente distribuído receptor.
func (d *Distribuido) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hmac.Equal([]byte(r.Header.Get("Authorization")), []byte("Bearer "+d.token)) {
			http.Error(w, "acesso não autorizado", 401)
			return
		}
		marca, err := strconv.ParseUint(r.Header.Get(cabecalhoLamport), 10, 63)
		if err != nil || marca == 0 {
			http.Error(w, "relogio de Lamport ausente ou invalido", 400)
			return
		}
		d.evento(marca)
		acao := strings.TrimPrefix(r.URL.Path, "/distribuido/")
		if acao == "saude" && r.Method == http.MethodGet {
			d.responderCoordenacao(w, saudeEmpresa{d.id, d.geracao}, nil)
			return
		}
		if acao == "estado" && r.Method == http.MethodGet {
			b, _ := json.Marshal(protocolo.Identificador{ID: r.URL.Query().Get("id")})
			v, err := d.executar("estado", b)
			d.responderCoordenacao(w, v, err)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "método inválido", 405)
			return
		}
		if acao != "preparar" && acao != "finalizar" && acao != "cancelar" {
			http.NotFound(w, r)
			return
		}
		b, err := io.ReadAll(io.LimitReader(r.Body, protocolo.LimiteMensagem+1))
		if err != nil || len(b) > protocolo.LimiteMensagem {
			http.Error(w, "mensagem excede limite", 413)
			return
		}
		v, err := d.executar(acao, b)
		d.responderCoordenacao(w, v, err)
	})
}

// executar persiste preparacoes, decisoes e aplicacoes antes de responder.
// Recebe: ação e bytes JSON.
func (d *Distribuido) executar(acao string, b []byte) (any, error) {
	marca := d.evento(0)
	resultado, err := d.executarLocal(acao, b)
	if acao != "saude" {
		if backupErr := d.g.SalvarBackup(); backupErr != nil {
			return nil, backupErr
		}
		var envelope struct {
			Meta metaTransacao
			ID   string `json:"id"`
		}
		_ = json.Unmarshal(b, &envelope)
		transacao := envelope.Meta.ID
		if transacao == "" {
			transacao = envelope.ID
		}
		var ordem uint64
		if acao == "preparar" {
			d.g.mu.RLock()
			if tx := d.g.transacoes[envelope.Meta.ID]; tx != nil {
				ordem = tx.ordem
			}
			d.g.mu.RUnlock()
		}
		d.g.logger.Printf("2PC empresa=%s transacao=%s etapa=%s lamport=%d ordem_local=%d sucesso=%t", d.id, transacao, acao, marca, ordem, err == nil)
	}
	return resultado, err
}

// executarLocal despacha saude, preparacao, decisao, consulta e cancelamento 2PC.
// Recebe: ação e bytes JSON.
func (d *Distribuido) executarLocal(acao string, b []byte) (any, error) {
	switch acao {
	case "saude":
		return saudeEmpresa{d.id, d.geracao}, nil
	case "preparar":
		var p prepararRPC
		if err := decodificar(b, &p); err != nil {
			return nil, err
		}
		return d.preparar(p)
	case "estado":
		var p protocolo.Identificador
		if err := decodificar(b, &p); err != nil {
			return nil, err
		}
		if p.ID == "" || len(p.ID) > 128 {
			return nil, fmt.Errorf("transacao invalida")
		}
		return d.estado(p.ID)
	case "cancelar":
		var p pedidoCancelamento
		if err := decodificar(b, &p); err != nil {
			return nil, err
		}
		d.mu.Lock()
		r := copiaJSON(d.registros[p.ID])
		d.mu.Unlock()
		if r == nil || r.Meta.Origem != d.id || r.Meta.Tipo != "RESERVAR" || r.Final == nil || r.Final.Valor.Estado != "CONFIRMAR" {
			return nil, fmt.Errorf("reserva nao confirmada neste coordenador")
		}
		err := d.cancelarGlobal(context.Background(), r.Meta, p.Motivo)
		return resultadoRPC{}, err
	case "finalizar":
		var c decisaoFinal
		if err := decodificar(b, &c); err != nil {
			return nil, err
		}
		return d.finalizar(c)
	}
	return nil, fmt.Errorf("ação desconhecida")
}

// chamar usa a mesma operacao local ou a API REST do par, com prazo de rede.
// Recebe: contexto, empresa, ação, pedido e ponteiro alvo.
func (d *Distribuido) chamar(ctx context.Context, id, acao string, p, alvo any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	envio := d.evento(0)
	if err := d.g.SalvarBackup(); err != nil {
		return err
	}
	if id == d.id {
		d.evento(envio)
		v, e := d.executar(acao, b)
		if e != nil {
			return e
		}
		resposta := d.evento(0)
		d.evento(resposta)
		if err := d.g.SalvarBackup(); err != nil {
			return err
		}
		b, e = json.Marshal(v)
		if e != nil {
			return e
		}
		return json.Unmarshal(b, alvo)
	}
	endereco := d.pares[id]
	if endereco == "" {
		return fmt.Errorf("empresa desconhecida")
	}
	metodo := http.MethodPost
	if acao == "saude" {
		metodo = http.MethodGet
	}
	caminho := endereco + "/distribuido/" + acao
	if acao == "estado" {
		var p protocolo.Identificador
		if err := json.Unmarshal(b, &p); err != nil {
			return err
		}
		metodo = http.MethodGet
		caminho += "?id=" + url.QueryEscape(p.ID)
	}
	var corpo io.Reader = bytes.NewReader(b)
	if metodo == http.MethodGet {
		corpo = nil
	}
	req, err := http.NewRequestWithContext(ctx, metodo, caminho, corpo)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.token)
	req.Header.Set(cabecalhoLamport, strconv.FormatUint(envio, 10))
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.cliente.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if s := resp.Header.Get(cabecalhoLamport); s != "" || resp.StatusCode == 200 {
		marca, err := strconv.ParseUint(s, 10, 63)
		if err != nil || marca == 0 {
			return fmt.Errorf("resposta sem relogio de Lamport valido")
		}
		d.evento(marca)
		if err := d.g.SalvarBackup(); err != nil {
			return err
		}
	}
	b, err = io.ReadAll(io.LimitReader(resp.Body, protocolo.LimiteMensagem+1))
	if err != nil {
		return err
	}
	if len(b) > protocolo.LimiteMensagem {
		return fmt.Errorf("resposta excede limite")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s: %s", id, strings.TrimSpace(string(b)))
	}
	return json.Unmarshal(b, alvo)
}

// geracoes consulta a identidade do estado de cada empresa; nao ha votacao.
// Recebe: contexto e lista de membros d.
func (d *Distribuido) geracoes(ctx context.Context) map[string]string {
	var mu sync.Mutex
	var wg sync.WaitGroup
	gs := make(map[string]string)
	for _, id := range d.membros {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			var s saudeEmpresa
			if d.chamar(ctx, id, "saude", nil, &s) == nil && s.Empresa == id {
				mu.Lock()
				gs[id] = s.Geracao
				mu.Unlock()
			}
		}(id)
	}
	wg.Wait()
	return gs
}
