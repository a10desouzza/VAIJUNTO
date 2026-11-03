// Autor: Arthur de Andrade Souza.
// Matéria: TEC502 - TP01 - Concorrência e Conectividade - 2026.2.
// Declaração de não plágio: Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
// Funcionalidade: Define registros, travas e autenticação do 2PC, mantém Lamport e propaga decisões aos envolvidos.
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

type metaTransacao struct {
	Lamport       uint64            `json:"lamport,omitempty"`
	ID            string            `json:"transacao_id"`
	Origem        string            `json:"empresa_origem"`
	Inicio        string            `json:"inicio_itinerario"`
	Tipo          string            `json:"tipo"`
	Original      string            `json:"transacao_original,omitempty"`
	Participantes []string          `json:"participantes"`
	Hashes        map[string]string `json:"hashes"`
	Geracoes      map[string]string `json:"geracoes"`
}

type decisao struct {
	Estado  string            `json:"estado"`
	Motivo  string            `json:"motivo,omitempty"`
	Recibos map[string]string `json:"recibos,omitempty"`
}

type decisaoFinal struct {
	Meta       metaTransacao `json:"meta"`
	Valor      decisao       `json:"valor"`
	Assinatura string        `json:"assinatura"`
}

type registroTransacao struct {
	Meta     metaTransacao
	Final    *decisaoFinal
	Aplicado bool
	Criado   time.Time
}

type operacaoOrigem struct {
	Meta                   metaTransacao
	Assinatura, Passageiro string
	Reserva                protocolo.Reserva
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

type Distribuido struct {
	g                          *GrafoItinerarios
	id, token, geracao         string
	pares                      map[string]string
	membros                    []string
	cliente                    *http.Client
	catalogo                   *consorcio.API
	mu                         sync.Mutex
	sequencia                  uint64
	relogio                    uint64
	registros                  map[string]*registroTransacao
	origens                    map[string]*operacaoOrigem
	locais, decisoes, chamadas sync.Map
	entregas                   sync.Map
	observadas                 sync.Map
	limite                     chan struct{}
	abortarApos                time.Duration

	aposPreparar func() error
	aposDecidir  func() error
}

// evento avanca o relogio logico: evento local/envio L+1; recepcao max(L,R)+1. Nao consulta o horario fisico e nao concede prioridade sobre assentos.
// Recebe: marca recebida, ou zero para evento local/envio.
func (d *Distribuido) evento(recebido uint64) uint64 {
	d.mu.Lock()
	defer d.mu.Unlock()
	if recebido > d.relogio {
		d.relogio = recebido
	}
	d.relogio++
	return d.relogio
}

// NovoDistribuido configura a empresa e seus dois pares; nao abre portas.
// Recebe: estado local, identidade, token, dois pares, catálogo e timeout.
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
		registros:   make(map[string]*registroTransacao),
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
// Recebe: mapa de mutexes e ID da operação.
func travarOperacao(m *sync.Map, id string) func() {
	v, _ := m.LoadOrStore(id, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// copiaJSON separa mapas e listas dos registros internos antes de liberar a trava.
// Recebe: valor serializável pelos modelos internos.
func copiaJSON[T any](v T) T {
	dados, _ := json.Marshal(v)
	var copia T
	_ = json.Unmarshal(dados, &copia)
	return copia
}

// hashJSON calcula o resumo SHA-256 da representacao JSON dos metadados.
// Recebe: valor serializável.
func hashJSON(v any) string {
	b, _ := json.Marshal(v)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// assinar calcula HMAC-SHA256 com o token do consorcio para autenticar metadados de coordenacao.
// Recebe: valor serializável e token do componente d.
func (d *Distribuido) assinar(v any) string {
	b, _ := json.Marshal(v)
	h := hmac.New(sha256.New, []byte(d.token))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

// recibo produz a autenticacao da preparacao de um participante para os metadados informados.
// Recebe: metadados e ID do participante.
func (d *Distribuido) recibo(m metaTransacao, id string) string {
	return d.assinar([]string{"preparada", m.ID, id, m.Hashes[id], m.Geracoes[id]})
}

// contem verifica se um identificador pertence a lista recebida.
// Recebe: lista de IDs e ID procurado.
func contem(ids []string, id string) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// propagar repete a fase 2 nos donos e na origem; um terceiro sem trechos nao e necessario.
// Recebe: contexto e decisão final.
func (d *Distribuido) propagar(ctx context.Context, c decisaoFinal) map[string]resultadoRPC {
	var mu sync.Mutex
	var wg sync.WaitGroup
	rs := make(map[string]resultadoRPC)

	if d.g.SalvarBackup() != nil {
		return rs
	}
	for _, id := range destinos(c.Meta) {
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
