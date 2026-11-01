// Arquivo: internal/servidor/distribuido.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Define o estado e as mensagens necessários à coordenação entre as três empresas.
// Valida preparações e certificados e aplica a decisão nos assentos locais.
// Retransmite decisões e recupera transações interrompidas sem um coordenador fixo.
// Somente uma decisão certificada autoriza liberar preparações; timeout isolado não basta.

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
// Recebe: b cedula.
// Retorna: true se a cedula receptora supera a recebida por numero ou, no empate, por ID; false caso contrario.
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
// Recebe: g *GrafoItinerarios, id, token string, pares []configuracao.Empresa, api *consorcio.API, timeout time.Duration.
// Retorna: o coordenador local do protocolo ou erro de configuracao; nao abre escutas.
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
// Recebe: m *sync.Map, id string.
// Retorna: uma funcao que libera as travas adquiridas; o chamador deve executa-la ao terminar a operacao protegida.
func travarOperacao(m *sync.Map, id string) func() {
	v, _ := m.LoadOrStore(id, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// copiaJSON separa mapas e listas dos registros internos antes de liberar a trava.
// Recebe: v T.
// Retorna: uma copia dos dados serializaveis para evitar compartilhar mapas e listas internos; requer tipos internos compativeis com JSON.
func copiaJSON[T any](v T) T {
	dados, _ := json.Marshal(v)
	var copia T
	_ = json.Unmarshal(dados, &copia)
	return copia
}

// hashJSON calcula o resumo SHA-256 da representacao JSON dos metadados.
// Recebe: v any.
// Retorna: string.
func hashJSON(v any) string {
	b, _ := json.Marshal(v)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// assinar calcula HMAC-SHA256 com o token do consorcio para autenticar metadados de coordenacao.
// Recebe: v any.
// Retorna: string.
func (d *Distribuido) assinar(v any) string {
	b, _ := json.Marshal(v)
	h := hmac.New(sha256.New, []byte(d.token))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

// recibo produz a autenticacao da preparacao de um participante para os metadados informados.
// Recebe: m metaTransacao, id string.
// Retorna: string.
func (d *Distribuido) recibo(m metaTransacao, id string) string {
	return d.assinar([]string{"preparada", m.ID, id, m.Hashes[id], m.Geracoes[id]})
}

// voto produz a autenticacao de um voto para a cedula, decisao e empresa informadas.
// Recebe: m metaTransacao, b cedula, v decisao, id string.
// Retorna: string.
func (d *Distribuido) voto(m metaTransacao, b cedula, v decisao, id string) string {
	return d.assinar(struct {
		Meta    metaTransacao
		Cedula  cedula
		Valor   decisao
		Empresa string
	}{m, b, v, id})
}

// contem verifica se um identificador pertence a lista recebida.
// Recebe: ids []string, id string.
// Retorna: true se o identificador aparece na lista; false caso contrario.
func contem(ids []string, id string) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// validarMeta confere participantes e a geracao que pode continuar esta transacao.
// Recebe: m metaTransacao.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
func (d *Distribuido) validarMeta(m metaTransacao) error {
	if m.ID == "" || len(m.ID) > 128 || (m.Tipo != "RESERVAR" && m.Tipo != "CANCELAR") || !contem(d.membros, m.Origem) {
		return fmt.Errorf("metadados inválidos")
	}
	if m.Geracoes[d.id] != d.geracao {
		return fmt.Errorf("geração do processo perdida; operação antiga não pode ser reaplicada")
	}
	if _, err := time.Parse(time.RFC3339, m.Inicio); err != nil {
		return fmt.Errorf("início inválido")
	}
	if len(m.Participantes) < 1 || len(m.Participantes) > 3 {
		return fmt.Errorf("participantes inválidos")
	}
	vistos := make(map[string]bool)
	for _, id := range m.Participantes {
		if vistos[id] || !contem(d.membros, id) || m.Hashes[id] == "" {
			return fmt.Errorf("participante inválido")
		}
		vistos[id] = true
	}
	if m.Tipo == "CANCELAR" && m.Original == "" {
		return fmt.Errorf("transação original obrigatória")
	}
	return nil
}

// validarDecisao exige o recibo de todos os donos antes de aceitar uma confirmacao.
// Recebe: m metaTransacao, v decisao.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
func (d *Distribuido) validarDecisao(m metaTransacao, v decisao) error {
	if v.Estado == "ABORTAR" {
		return nil
	}
	if v.Estado != "CONFIRMAR" {
		return fmt.Errorf("decisão inválida")
	}
	for _, id := range m.Participantes {
		if !hmac.Equal([]byte(v.Recibos[id]), []byte(d.recibo(m, id))) {
			return fmt.Errorf("faltou preparação de %s", id)
		}
	}
	return nil
}

// preparar registra a operacao e separa somente os assentos da empresa local.
// Recebe: p prepararRPC.
// Retorna: o recibo de preparacao ou erro. O recibo so e enviado pelo atendimento depois do checkpoint local.
func (d *Distribuido) preparar(p prepararRPC) (resultadoRPC, error) {
	if err := d.validarMeta(p.Meta); err != nil {
		return resultadoRPC{}, err
	}
	mesmaTransacao := p.Pedido.TransacaoID == p.Meta.ID && p.Pedido.EmpresaOrigem == p.Meta.Origem
	mesmoItinerario := p.Pedido.InicioItinerario == p.Meta.Inicio && hashJSON(p.Pedido.Participantes) == hashJSON(p.Meta.Participantes)
	pedidoIntegro := p.Meta.Hashes[d.id] == hashJSON(p.Pedido)
	if !contem(p.Meta.Participantes, d.id) || !mesmaTransacao || !mesmoItinerario || !pedidoIntegro {
		return resultadoRPC{}, fmt.Errorf("pedido diverge dos metadados")
	}
	liberar := travarOperacao(&d.locais, p.Meta.ID)
	defer liberar()
	d.mu.Lock()
	r := d.registros[p.Meta.ID]
	if r == nil {
		r = &registroConsenso{Meta: p.Meta, Criado: time.Now()}
		d.registros[p.Meta.ID] = r
	}
	if hashJSON(r.Meta) != hashJSON(p.Meta) || r.Final != nil {
		d.mu.Unlock()
		return resultadoRPC{}, fmt.Errorf("transação divergente ou já decidida")
	}
	d.mu.Unlock()
	if p.Meta.Tipo == "CANCELAR" {
		d.g.mu.RLock()
		t := d.g.transacoes[p.Meta.Original]
		ok := t != nil && t.estado == protocolo.TransacaoConfirmada
		d.g.mu.RUnlock()
		inicio, _ := time.Parse(time.RFC3339, p.Meta.Inicio)
		if !ok || !inicio.After(d.g.agora()) {
			return resultadoRPC{}, fmt.Errorf("reserva não encontrada ou viagem iniciada")
		}
	} else {
		r, err := d.g.prepararTransacao(p.Pedido)
		if err != nil {
			return resultadoRPC{}, err
		}
		if r.Estado != protocolo.TransacaoPreparada {
			return resultadoRPC{}, fmt.Errorf("transação não está preparada")
		}
	}
	return resultadoRPC{Recibo: d.recibo(p.Meta, d.id)}, nil
}

// resultadoLocal obtem uma copia da reserva local associada a transacao, se ela existir.
// Recebe: id string.
// Retorna: resultadoRPC.
func (d *Distribuido) resultadoLocal(id string) resultadoRPC {
	d.g.mu.RLock()
	defer d.g.mu.RUnlock()
	t := d.g.transacoes[id]
	if t == nil {
		return resultadoRPC{}
	}
	return resultadoRPC{Reserva: copiarReserva(d.g.reservas[t.reservaID])}
}

// finalizar valida o certificado e aplica a decisao uma unica vez.
// Recebe: c certificado.
// Retorna: o resultado local da decisao certificada ou erro de validacao/aplicacao; repeticoes preservam a decisao.
func (d *Distribuido) finalizar(c certificado) (resultadoRPC, error) {
	if err := d.validarMeta(c.Meta); err != nil {
		return resultadoRPC{}, err
	}
	if err := d.validarDecisao(c.Meta, c.Valor); err != nil {
		return resultadoRPC{}, err
	}
	quantidade := 0
	for id, v := range c.Votos {
		if contem(d.membros, id) && c.Meta.Geracoes[id] != "" && hmac.Equal([]byte(v), []byte(d.voto(c.Meta, c.Cedula, c.Valor, id))) {
			quantidade++
		}
	}
	if quantidade < 2 {
		return resultadoRPC{}, fmt.Errorf("certificado sem maioria")
	}
	liberar := travarOperacao(&d.locais, c.Meta.ID)
	defer liberar()
	d.mu.Lock()
	r := d.registros[c.Meta.ID]
	if r == nil {
		r = &registroConsenso{Meta: c.Meta, Criado: time.Now()}
		d.registros[c.Meta.ID] = r
	}
	if hashJSON(r.Meta) != hashJSON(c.Meta) || r.Final != nil && hashJSON(r.Final.Valor) != hashJSON(c.Valor) {
		d.mu.Unlock()
		return resultadoRPC{}, fmt.Errorf("decisão conflitante")
	}
	if r.Aplicado {
		d.mu.Unlock()
		id := c.Meta.ID
		if c.Meta.Tipo == "CANCELAR" {
			id = c.Meta.Original
		}
		return d.resultadoLocal(id), nil
	}
	cp := copiaJSON(c)
	r.Final = &cp
	d.mu.Unlock()
	var reserva protocolo.Reserva
	var err error
	if contem(c.Meta.Participantes, d.id) {
		if c.Meta.Tipo == "CANCELAR" {
			if c.Valor.Estado == "CONFIRMAR" {
				reserva, err = d.g.cancelarTransacaoConfirmada(c.Meta.Original, c.Valor.Motivo)
			}
		} else if c.Valor.Estado == "CONFIRMAR" {
			reserva, err = d.g.confirmarTransacao(c.Meta.ID)
		} else {
			d.g.mu.RLock()
			existe := d.g.transacoes[c.Meta.ID] != nil
			d.g.mu.RUnlock()
			if existe {
				_, err = d.g.abortarTransacao(c.Meta.ID, c.Valor.Motivo)
			}
		}
	}
	if err != nil {
		return resultadoRPC{}, err
	}
	d.mu.Lock()
	r.Aplicado = true
	var aviso *protocolo.Reserva
	if c.Meta.Tipo == "CANCELAR" && c.Valor.Estado == "CONFIRMAR" {
		if op := d.origens[c.Meta.Original]; op != nil && op.Reserva.Status != protocolo.Cancelada {
			op.Reserva.Status = protocolo.Cancelada
			op.Reserva.Motivo = c.Valor.Motivo
			op.Reserva.CanceladaEm = time.Now().UTC().Format(time.RFC3339)
			cp := copiarReserva(op.Reserva)
			aviso = &cp
		}
	}
	d.mu.Unlock()
	if aviso != nil {
		d.g.mu.Lock()
		d.g.notificar(*aviso)
		d.g.mu.Unlock()
	}
	return resultadoRPC{Reserva: reserva}, nil
}

// propagar entrega a decisao as testemunhas que pertencem a geracao da operacao.
// Recebe: ctx context.Context, c certificado.
// Retorna: um mapa com os resultados apenas das empresas que responderam com sucesso; ausentes podem exigir nova entrega.
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
// Recebe: ctx context.Context.
// Retorna: nenhum valor; executa os efeitos descritos acima.
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
// Recebe: ctx context.Context.
// Retorna: nenhum valor; executa os efeitos descritos acima.
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
