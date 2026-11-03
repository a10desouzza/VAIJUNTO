// Autor: Arthur de Andrade Souza.
// Matéria: TEC502 - TP01 - Concorrência e Conectividade - 2026.2.
// Declaração de não plágio: Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
// Funcionalidade: Registra a decisão imutável do 2PC e recupera operações pendentes em ordem lógica de Lamport.
package servidor

import (
	"context"
	"fmt"
	"sort"
	"time"
	"vaijunto/internal/protocolo"
)

// decidir e a fase 2 do 2PC: so a origem pode escolher e gravar o resultado. Depois de gravado, o resultado e imutavel e pode ser reenviado apos uma queda.
// Recebe: contexto, metadados e proposta CONFIRMAR/ABORTAR.
func (d *Distribuido) decidir(ctx context.Context, m metaTransacao, valor decisao) (decisaoFinal, error) {
	if m.Origem != d.id {
		return decisaoFinal{}, fmt.Errorf("somente o coordenador da transacao decide")
	}
	if err := d.validarMeta(m); err != nil {
		return decisaoFinal{}, err
	}
	if err := ctx.Err(); err != nil {
		return decisaoFinal{}, err
	}
	liberar := travarOperacao(&d.decisoes, m.ID)
	defer liberar()
	d.evento(0)
	d.mu.Lock()
	r := d.registros[m.ID]
	if r == nil || hashJSON(r.Meta) != hashJSON(m) {
		d.mu.Unlock()
		return decisaoFinal{}, fmt.Errorf("transacao nao registrada pelo coordenador")
	}
	nova := r.Final == nil
	if nova {
		if err := d.validarDecisao(m, valor); err != nil {
			d.mu.Unlock()
			return decisaoFinal{}, err
		}
		c := decisaoFinal{Meta: m, Valor: valor, Assinatura: d.assinar([]any{m, valor})}
		r.Final = &c
	}
	c := copiaJSON(*r.Final)
	d.mu.Unlock()

	if err := d.g.SalvarBackup(); err != nil {
		return decisaoFinal{}, err
	}
	if nova && d.aposDecidir != nil {
		if err := d.aposDecidir(); err != nil {
			return decisaoFinal{}, err
		}
	}
	return c, nil
}

type respostaEstado struct {
	Final *decisaoFinal `json:"decisao,omitempty"`
}

type pedidoCancelamento struct {
	ID     string `json:"id"`
	Motivo string `json:"motivo"`
}

// estado responde a consulta de recuperacao; desconhecido nao significa abortado. executar salva o backup antes de enviar esta resposta pela rede.
// Recebe: ID da transação no coordenador.
func (d *Distribuido) estado(id string) (respostaEstado, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	r := d.registros[id]
	if r == nil {
		return respostaEstado{}, nil
	}
	if r.Meta.Origem != d.id {
		return respostaEstado{}, fmt.Errorf("consulte o coordenador da transacao")
	}
	return respostaEstado{Final: copiaJSON(r.Final)}, nil
}

// destinos inclui apenas os donos e a origem, sem testemunhas nem quorum.
// Recebe: metadados da transação.
func destinos(m metaTransacao) []string {
	vistos := map[string]bool{m.Origem: true}
	for _, id := range m.Participantes {
		vistos[id] = true
	}
	ids := make([]string, 0, len(vistos))
	for id := range vistos {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Recuperar executa a recuperacao periodica ate o encerramento do servidor.
// Recebe: contexto de execução e estado d.
func (d *Distribuido) Recuperar(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.recuperar(ctx)
		}
	}
}

// recuperar percorre registros em ordem lógica, reenvia decisões e consulta coordenadores de operações pendentes.
// Recebe: contexto e registros locais d.
func (d *Distribuido) recuperar(ctx context.Context) {
	d.mu.Lock()
	registros := copiaJSON(d.registros)
	d.mu.Unlock()
	ids := make([]string, 0, len(registros))
	for id := range registros {
		ids = append(ids, id)
	}

	sort.Slice(ids, func(i, j int) bool {
		a, b := registros[ids[i]].Meta, registros[ids[j]].Meta
		if a.Lamport != b.Lamport {
			return a.Lamport < b.Lamport
		}
		if a.Origem != b.Origem {
			return a.Origem < b.Origem
		}
		return a.ID < b.ID
	})
	for _, id := range ids {
		r := registros[id]
		if ctx.Err() != nil {
			return
		}
		if r.Final != nil {
			if _, ok := d.entregas.Load(id); !ok {
				if len(d.propagar(ctx, *r.Final)) == len(destinos(r.Meta)) {
					d.entregas.Store(id, true)
				}
			}
			continue
		}
		if r.Meta.Origem == d.id {
			inicio, _ := d.observadas.LoadOrStore(id, time.Now())
			if time.Since(inicio.(time.Time)) < d.abortarApos {
				continue
			}

			liberar := travarOperacao(&d.chamadas, id)
			c, err := d.decidir(ctx, r.Meta, decisao{Estado: "ABORTAR", Motivo: "coordenador recuperou operacao sem decisao"})
			liberar()
			if err == nil {
				d.propagar(ctx, c)
			}
		} else {

			var resposta respostaEstado
			err := d.chamar(ctx, r.Meta.Origem, "estado", protocolo.Identificador{ID: id}, &resposta)
			if err == nil && resposta.Final != nil && hashJSON(resposta.Final.Meta) == hashJSON(r.Meta) {
				d.propagar(ctx, *resposta.Final)
			}
		}
	}
	d.atualizarOrigens(ctx)
}
