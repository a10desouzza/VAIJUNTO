// Arquivo: internal/consorcio/rest.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Publica o catálogo de trechos disponíveis da própria empresa por API REST.
// Consulta os catálogos dos pares em paralelo e identifica empresas sem resposta.
// Esses resultados alimentam a busca distribuída sem manter uma base remota replicada.

package consorcio

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"vaijunto/internal/configuracao"
	"vaijunto/internal/protocolo"
)

// TrechoPublico omite dados pessoais e identifica explicitamente o dono.
type TrechoPublico struct {
	Capacidade  int     `json:"capacidade"`
	EmpresaID   string  `json:"empresa_id"`
	ID          string  `json:"trecho_id"`
	CaronaID    string  `json:"carona_id"`
	Origem      string  `json:"origem"`
	Destino     string  `json:"destino"`
	DataHora    string  `json:"data_hora"`
	Assentos    int     `json:"assentos_livres"`
	Preco       float64 `json:"preco"`
	DistanciaKM float64 `json:"distancia_km"`
	TempoViagem int     `json:"tempo_viagem_min"`
	TempoParada int     `json:"tempo_parada_min"`
}

type Catalogo struct {
	EmpresaID string          `json:"empresa_id"`
	Trechos   []TrechoPublico `json:"trechos"`
}

type Consulta struct {
	Trechos       []TrechoPublico `json:"trechos"`
	Indisponiveis []string        `json:"empresas_indisponiveis"`
}

type Fonte interface{ TrechosDisponiveis() []protocolo.Trecho }

type API struct {
	id, token string
	empresas  []configuracao.Empresa
	fonte     Fonte
	cliente   *http.Client
}

// NovaAPI configura identidade, token, pares e fonte local; rejeita configuracoes invalidas.
// Recebe: id, token string, empresas []configuracao.Empresa, fonte Fonte, timeout time.Duration.
// Retorna: a API de catalogos configurada ou erro de configuracao; nao abre a escuta HTTP.
func NovaAPI(id, token string, empresas []configuracao.Empresa, fonte Fonte, timeout time.Duration) (*API, error) {
	if err := configuracao.ValidarEmpresaID(id); err != nil {
		return nil, err
	}
	if len(token) < 16 {
		return nil, fmt.Errorf("token do consórcio deve ter pelo menos 16 caracteres")
	}
	if fonte == nil || timeout <= 0 {
		return nil, fmt.Errorf("fonte e timeout positivos são obrigatórios")
	}
	return &API{id: id, token: token, empresas: append([]configuracao.Empresa(nil), empresas...), fonte: fonte,
		cliente: &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}}}, nil
}

// local monta o catalogo publico com os trechos disponiveis da propria empresa, sem dados pessoais.
// Recebe: nenhum parametro.
// Retorna: Catalogo.
func (a *API) local() Catalogo {
	c := Catalogo{EmpresaID: a.id, Trechos: make([]TrechoPublico, 0)}
	for _, t := range a.fonte.TrechosDisponiveis() {
		c.Trechos = append(c.Trechos, TrechoPublico{Capacidade: t.Capacidade, EmpresaID: a.id, ID: t.ID, CaronaID: t.CaronaID, Origem: t.Origem, Destino: t.Destino, DataHora: t.DataHora, Assentos: t.Assentos, Preco: t.Preco, DistanciaKM: t.DistanciaKM, TempoViagem: t.TempoViagem, TempoParada: t.TempoParada})
	}
	return c
}

// consultar consulta o catalogo REST do par recebido, validando limite da resposta e identidade da empresa.
// Recebe: ctx context.Context, empresa configuracao.Empresa.
// Retorna: o resultado da consulta ou erro de acesso/validacao, conforme o recurso deste pacote.
func (a *API) consultar(ctx context.Context, empresa configuracao.Empresa) (Catalogo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, empresa.URL+"/trechos", nil)
	if err != nil {
		return Catalogo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	resp, err := a.cliente.Do(req)
	if err != nil {
		return Catalogo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Catalogo{}, fmt.Errorf("empresa %s retornou HTTP %d", empresa.ID, resp.StatusCode)
	}
	const limite = 8 << 20
	b, err := io.ReadAll(io.LimitReader(resp.Body, limite+1))
	if err != nil {
		return Catalogo{}, err
	}
	if len(b) > limite {
		return Catalogo{}, fmt.Errorf("catálogo excedeu o limite")
	}
	var c Catalogo
	if err := json.Unmarshal(b, &c); err != nil {
		return Catalogo{}, err
	}
	if c.EmpresaID != empresa.ID {
		return Catalogo{}, fmt.Errorf("identidade da empresa divergente")
	}
	ids := make(map[string]bool)
	for _, t := range c.Trechos {
		if t.EmpresaID != empresa.ID || t.ID == "" || ids[t.ID] {
			return Catalogo{}, fmt.Errorf("referência de trecho inválida")
		}
		ids[t.ID] = true
	}
	return c, nil
}

// ConsultarTodos consulta somente /trechos dos pares, evitando chamadas recursivas.
// Uma empresa indisponivel nao impede o retorno das ofertas das demais.
// Recebe: ctx context.Context.
// Retorna: Consulta.
func (a *API) ConsultarTodos(ctx context.Context) Consulta {
	resultado := Consulta{Trechos: a.local().Trechos, Indisponiveis: make([]string, 0)}
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, empresa := range a.empresas {
		wg.Add(1)
		go func(empresa configuracao.Empresa) {
			defer wg.Done()
			c, err := a.consultar(ctx, empresa)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				resultado.Indisponiveis = append(resultado.Indisponiveis, empresa.ID)
				return
			}
			resultado.Trechos = append(resultado.Trechos, c.Trechos...)
		}(empresa)
	}
	wg.Wait()
	sort.Strings(resultado.Indisponiveis)
	sort.Slice(resultado.Trechos, func(i, j int) bool {
		x, y := resultado.Trechos[i], resultado.Trechos[j]
		if x.EmpresaID != y.EmpresaID {
			return x.EmpresaID < y.EmpresaID
		}
		return x.ID < y.ID
	})
	return resultado
}

// responder define o tipo JSON e escreve a representacao do valor na resposta HTTP.
// Recebe: w http.ResponseWriter, valor any.
// Retorna: nenhum valor; executa os efeitos descritos acima.
func responder(w http.ResponseWriter, valor any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(valor)
}

// Handler constroi o atendimento HTTP dos recursos de consulta com autenticacao pelo token do consorcio.
// Recebe: nenhum parametro.
// Retorna: um http.Handler que recebe as requisicoes; a escuta e iniciada pelo executavel do servidor.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /saude", func(w http.ResponseWriter, r *http.Request) {
		responder(w, map[string]string{"empresa_id": a.id, "status": "ok"})
	})
	mux.HandleFunc("GET /trechos", func(w http.ResponseWriter, r *http.Request) {
		responder(w, a.local())
	})
	mux.HandleFunc("GET /consorcio/trechos", func(w http.ResponseWriter, r *http.Request) {
		responder(w, a.ConsultarTodos(r.Context()))
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+a.token)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "acesso não autorizado", http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// AgruparTrechos separa as referencias por dono sem alterar a ordem do pedido.
// Recebe: pedido protocolo.ReservaDistribuida.
// Retorna: (map[string][]string, error). Em falha, devolve error; em sucesso, erro nil.
func AgruparTrechos(pedido protocolo.ReservaDistribuida) (map[string][]string, error) {
	if strings.TrimSpace(pedido.Chave) == "" || len(pedido.Chave) > 100 {
		return nil, fmt.Errorf("chave de reserva ausente")
	}

	if len(pedido.Trechos) < 1 || len(pedido.Trechos) > 20 {
		return nil, fmt.Errorf("informe de 1 a 20 trechos para a reserva")
	}

	grupos := make(map[string][]string)
	vistos := make(map[protocolo.ReferenciaTrecho]bool)

	for _, referencia := range pedido.Trechos {
		if err := configuracao.ValidarEmpresaID(referencia.EmpresaID); err != nil {
			return nil, err
		}

		if strings.TrimSpace(referencia.TrechoID) == "" {
			return nil, fmt.Errorf("trechoID ausente")
		}

		if vistos[referencia] {
			return nil, fmt.Errorf("trecho %s já informado", referencia.TrechoID)
		}

		vistos[referencia] = true

		grupos[referencia.EmpresaID] = append(grupos[referencia.EmpresaID], referencia.TrechoID)
	}
	return grupos, nil
}
