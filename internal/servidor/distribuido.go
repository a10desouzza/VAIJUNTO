// TEC502 - Problema 2. Autor: Arthur de Andrade Souza.
// Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
package servidor

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"
	"vaijunto/internal/configuracao"
	"vaijunto/internal/consorcio"
	"vaijunto/internal/protocolo"
)

const pendente = "PENDENTE"

// So metadados de coordenacao circulam entre testemunhas, nunca contas ou
// copias das bases de caronas. Cada testemunha salva seus registros no proprio backup.
type metaTransacao struct {
	ID            string            `json:"transacao_id"`
	Origem        string            `json:"empresa_origem"`
	Inicio        string            `json:"inicio_itinerario"`
	Tipo          string            `json:"tipo"`
	Original      string            `json:"transacao_original,omitempty"`
	Participantes []string          `json:"participantes"`
	Hashes        map[string]string `json:"hashes"`
	Geracoes      map[string]string `json:"geracoes"`
}

type cedula struct {
	Numero  uint64 `json:"numero"`
	Empresa string `json:"empresa_id"`
}

// maior compara cedulas por numero e, em empate, pelo identificador da empresa.
func (a cedula) maior(b cedula) bool {
	return a.Numero > b.Numero || a.Numero == b.Numero && a.Empresa > b.Empresa
}

type decisao struct {
	Estado  string            `json:"estado"`
	Motivo  string            `json:"motivo,omitempty"`
	Recibos map[string]string `json:"recibos,omitempty"`
}

type certificado struct {
	Meta   metaTransacao     `json:"meta"`
	Cedula cedula            `json:"cedula"`
	Valor  decisao           `json:"valor"`
	Votos  map[string]string `json:"votos"`
}

type registroConsenso struct {
	Meta             metaTransacao
	Promessa, Aceita cedula
	Valor            *decisao
	Final            *certificado
	Aplicado         bool
	Criado           time.Time
}

type operacaoOrigem struct {
	Meta                   metaTransacao
	Assinatura, Passageiro string
	Reserva                protocolo.Reserva
}

type mensagemConsenso struct {
	Meta   metaTransacao `json:"meta"`
	Cedula cedula        `json:"cedula"`
	Valor  *decisao      `json:"valor,omitempty"`
}

type respostaConsenso struct {
	Promessa cedula       `json:"promessa"`
	Aceita   cedula       `json:"aceita"`
	Valor    *decisao     `json:"valor,omitempty"`
	Final    *certificado `json:"final,omitempty"`
	Voto     string       `json:"voto,omitempty"`
}

type prepararRPC struct {
	Meta   metaTransacao              `json:"meta"`
	Pedido protocolo.PedidoPreparacao `json:"pedido"`
}

type resultadoRPC struct {
	Recibo  string            `json:"recibo,omitempty"`
	Reserva protocolo.Reserva `json:"reserva,omitzero"`
}

type saudeEmpresa struct {
	Empresa string `json:"empresa_id"`
	Geracao string `json:"geracao"`
}

// Distribuido implementa consenso de valor unico por transacao (Paxos basico)
// e aplica a decisao nos donos das vagas. Nao ha coordenador fixo.
// Reiniciar com backup preserva promessas e geracao. Sem ele, a nova geracao
// impede que uma testemunha que esqueceu seu historico vote em transacoes antigas.
type Distribuido struct {
	g                           *GrafoItinerarios
	id, token, geracao          string
	pares                       map[string]string
	membros                     []string
	cliente                     *http.Client
	catalogo                    *consorcio.API
	mu                          sync.Mutex
	relogio                     uint64
	registros                   map[string]*registroConsenso
	origens                     map[string]*operacaoOrigem
	locais, propostas, chamadas sync.Map
	entregas                    sync.Map
	limite                      chan struct{}
	abortarApos                 time.Duration
	// Pontos de injecao de falha, usados pelos testes sem alterar o protocolo.
	aposPreparar func() error
	aposQuorum   func() error
}

// NovoDistribuido configura as tres testemunhas e a identidade local da empresa.
func NovoDistribuido(g *GrafoItinerarios, id, token string, pares []configuracao.Empresa, api *consorcio.API, timeout time.Duration) (*Distribuido, error) {
	if len(pares) != 2 || timeout <= 0 || len(token) < 16 || api == nil {
		return nil, fmt.Errorf("configure três empresas, catálogo, token e timeout")
	}
	if err := configuracao.ValidarEmpresaID(id); err != nil {
		return nil, err
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	d := &Distribuido{
		g: g, id: id, token: token,
		geracao:     hex.EncodeToString(b),
		pares:       make(map[string]string),
		membros:     []string{id},
		catalogo:    api,
		registros:   make(map[string]*registroConsenso),
		origens:     make(map[string]*operacaoOrigem),
		limite:      make(chan struct{}, 64),
		abortarApos: 10 * time.Second,
		cliente: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
	for _, p := range pares {
		if p.ID == id || d.pares[p.ID] != "" {
			return nil, fmt.Errorf("empresa duplicada")
		}
		d.pares[p.ID] = p.URL
		d.membros = append(d.membros, p.ID)
	}
	sort.Strings(d.membros)
	g.distribuido = d
	return d, nil
}

// travarOperacao serializa chamadas da mesma operacao por identificador, sem bloquear outras transacoes.
func travarOperacao(m *sync.Map, id string) func() {
	v, _ := m.LoadOrStore(id, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// copiaJSON separa mapas e listas dos registros internos antes de liberar a trava.
func copiaJSON[T any](v T) T {
	dados, _ := json.Marshal(v)
	var copia T
	_ = json.Unmarshal(dados, &copia)
	return copia
}

// hashJSON calcula o resumo SHA-256 da representacao JSON dos metadados.
func hashJSON(v any) string {
	b, _ := json.Marshal(v)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// assinar calcula HMAC-SHA256 com o token do consorcio para autenticar metadados de coordenacao.
func (d *Distribuido) assinar(v any) string {
	b, _ := json.Marshal(v)
	h := hmac.New(sha256.New, []byte(d.token))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

// recibo produz a autenticacao da preparacao de um participante para os metadados informados.
func (d *Distribuido) recibo(m metaTransacao, id string) string {
	return d.assinar([]string{"preparada", m.ID, id, m.Hashes[id], m.Geracoes[id]})
}

// voto produz a autenticacao de um voto para a cedula, decisao e empresa informadas.
func (d *Distribuido) voto(m metaTransacao, b cedula, v decisao, id string) string {
	return d.assinar(struct {
		Meta    metaTransacao
		Cedula  cedula
		Valor   decisao
		Empresa string
	}{m, b, v, id})
}

// contem verifica se um identificador pertence a lista recebida.
func contem(ids []string, id string) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// propagar entrega a decisao as testemunhas que pertencem a geracao da operacao.
func (d *Distribuido) propagar(ctx context.Context, c certificado) map[string]resultadoRPC {
	var mu sync.Mutex
	var wg sync.WaitGroup
	rs := make(map[string]resultadoRPC)
	for _, id := range d.membros {
		if c.Meta.Geracoes[id] == "" {
			continue
		}
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			var r resultadoRPC
			if d.chamar(ctx, id, "finalizar", c, &r) == nil {
				mu.Lock()
				rs[id] = r
				mu.Unlock()
			}
		}(id)
	}
	wg.Wait()
	return rs
}

// Timeouts apenas iniciam nova proposta; somente uma decisao certificada pode
// liberar vagas. Processos reiniciados com outra geracao recusam transacoes antigas.
func (d *Distribuido) Recuperar(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			d.recuperar(ctx)
		}
	}
}

// recuperar retoma decisoes pendentes sem liberar vagas por um timeout isolado.
func (d *Distribuido) recuperar(ctx context.Context) {
	d.mu.Lock()
	rs := copiaJSON(d.registros)
	d.mu.Unlock()
	for id, r := range rs {
		if ctx.Err() != nil {
			return
		}
		if r.Final != nil {
			if _, ok := d.entregas.Load(id); !ok {
				entregues := d.propagar(ctx, *r.Final)
				if len(entregues) == len(r.Meta.Geracoes) {
					d.entregas.Store(id, true)
				}
			}
			continue
		}
		if time.Since(r.Criado) < d.abortarApos {
			continue
		}
		c, err := d.decidir(ctx, r.Meta, decisao{Estado: "ABORTAR", Motivo: "operação interrompida; recuperação por maioria"})
		if err == nil {
			d.propagar(ctx, c)
		}
	}
	d.atualizarOrigens(ctx)
}
