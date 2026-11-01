// Arquivo: internal/servidor/rest_consorcio.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Recebe e envia mensagens REST de saúde, preparação, consenso e finalização.
// Autentica chamadas, limita mensagens e encaminha cada ação ao protocolo distribuído.
// Salva alterações antes de responder com sucesso e aplica prazos nas chamadas aos pares.

package servidor

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"vaijunto/internal/protocolo"
)

// Handler autentica e recebe as chamadas REST de coordenacao.
// Recebe: nenhum parametro.
// Retorna: um http.Handler que recebe as requisicoes; a escuta e iniciada pelo executavel do servidor.
func (d *Distribuido) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hmac.Equal([]byte(r.Header.Get("Authorization")), []byte("Bearer "+d.token)) {
			http.Error(w, "acesso não autorizado", 401)
			return
		}
		acao := strings.TrimPrefix(r.URL.Path, "/distribuido/")
		if acao == "saude" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(saudeEmpresa{d.id, d.geracao})
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "método inválido", 405)
			return
		}
		if acao != "preparar" && acao != "prometer" && acao != "aceitar" && acao != "finalizar" {
			http.NotFound(w, r)
			return
		}
		b, err := io.ReadAll(io.LimitReader(r.Body, protocolo.LimiteMensagem+1))
		if err != nil || len(b) > protocolo.LimiteMensagem {
			http.Error(w, "mensagem excede limite", 413)
			return
		}
		v, err := d.executar(acao, b)
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	})
}

// executar salva as alteracoes locais antes de devolver um recibo ou voto positivo.
// Recebe: acao string, b []byte.
// Retorna: (any, error). Em falha, devolve error; em sucesso, erro nil.
func (d *Distribuido) executar(acao string, b []byte) (any, error) {
	resultado, err := d.executarLocal(acao, b)
	if acao != "saude" {
		if backupErr := d.g.SalvarBackup(); backupErr != nil {
			return nil, backupErr
		}
		var envelope struct {
			Meta   metaTransacao
			Cedula cedula
		}
		_ = json.Unmarshal(b, &envelope)
		var ordem uint64
		if acao == "preparar" {
			d.g.mu.RLock()
			if tx := d.g.transacoes[envelope.Meta.ID]; tx != nil {
				ordem = tx.ordem
			}
			d.g.mu.RUnlock()
		}
		d.g.logger.Printf("COORDENACAO empresa=%s transacao=%s etapa=%s ordem_local=%d cedula=%d/%s sucesso=%t", d.id, envelope.Meta.ID, acao, ordem, envelope.Cedula.Numero, envelope.Cedula.Empresa, err == nil)
	}
	return resultado, err
}

// executarLocal despacha uma chamada de coordenacao para saude, preparacao, consenso ou finalizacao.
// Recebe: acao string, b []byte.
// Retorna: (any, error). Em falha, devolve error; em sucesso, erro nil.
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
	case "prometer", "aceitar":
		var p mensagemConsenso
		if err := decodificar(b, &p); err != nil {
			return nil, err
		}
		return d.consenso(acao, p)
	case "finalizar":
		var c certificado
		if err := decodificar(b, &c); err != nil {
			return nil, err
		}
		return d.finalizar(c)
	}
	return nil, fmt.Errorf("ação desconhecida")
}

// chamar usa a mesma operacao local ou a API REST do par, com prazo de rede.
// Recebe: ctx context.Context, id, acao string, p, alvo any.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
func (d *Distribuido) chamar(ctx context.Context, id, acao string, p, alvo any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if id == d.id {
		v, e := d.executar(acao, b)
		if e != nil {
			return e
		}
		b, e = json.Marshal(v)
		if e != nil {
			return e
		}
		return json.Unmarshal(b, alvo)
	}
	url := d.pares[id]
	if url == "" {
		return fmt.Errorf("empresa desconhecida")
	}
	metodo := http.MethodPost
	if acao == "saude" {
		metodo = http.MethodGet
	}
	req, err := http.NewRequestWithContext(ctx, metodo, url+"/distribuido/"+acao, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.cliente.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
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

// geracoes identifica as testemunhas disponiveis sem depender de relogios fisicos.
// Recebe: ctx context.Context.
// Retorna: um mapa de identidades para geracoes das testemunhas que responderam com identidade correta.
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
