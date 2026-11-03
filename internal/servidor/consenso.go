// TEC502 - Problema 2. Autor: Arthur de Andrade Souza.
// Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
package servidor

import (
	"context"
	"crypto/hmac"
	"fmt"
	"sync"
	"time"
)

// A maior cedula prometida impede aceitar decisoes concorrentes em cedulas
// antigas. O proponente deve adotar o valor aceito de maior cedula consultada.
func (d *Distribuido) consenso(acao string, p mensagemConsenso) (respostaConsenso, error) {
	if err := d.validarMeta(p.Meta); err != nil {
		return respostaConsenso{}, err
	}
	if p.Cedula.Numero == 0 || !contem(d.membros, p.Cedula.Empresa) {
		return respostaConsenso{}, fmt.Errorf("cédula inválida")
	}
	if acao == "aceitar" {
		if p.Valor == nil {
			return respostaConsenso{}, fmt.Errorf("valor ausente")
		}
		if err := d.validarDecisao(p.Meta, *p.Valor); err != nil {
			return respostaConsenso{}, err
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	r := d.registros[p.Meta.ID]
	if r == nil {
		r = &registroConsenso{Meta: p.Meta, Criado: time.Now()}
		d.registros[p.Meta.ID] = r
	}
	if hashJSON(r.Meta) != hashJSON(p.Meta) {
		return respostaConsenso{}, fmt.Errorf("metadados divergentes")
	}
	if p.Cedula.Numero > d.relogio {
		d.relogio = p.Cedula.Numero
	}
	if !r.Promessa.maior(p.Cedula) {
		r.Promessa = p.Cedula
		if acao == "aceitar" {
			if r.Aceita == p.Cedula && r.Valor != nil && hashJSON(r.Valor) != hashJSON(p.Valor) {
				return respostaConsenso{}, fmt.Errorf("cédula reutilizada")
			}
			if r.Final != nil && hashJSON(r.Final.Valor) != hashJSON(p.Valor) {
				return respostaConsenso{}, fmt.Errorf("decisão imutável")
			}
			v := *p.Valor
			r.Aceita = p.Cedula
			r.Valor = &v
		}
	}
	rs := respostaConsenso{Promessa: r.Promessa, Aceita: r.Aceita, Valor: r.Valor, Final: r.Final}
	if acao == "aceitar" && r.Aceita == p.Cedula {
		rs.Voto = d.voto(p.Meta, p.Cedula, *p.Valor, d.id)
	}
	return copiaJSON(rs), nil
}

// decidir busca maioria e preserva o valor ja aceito com a maior cedula.
func (d *Distribuido) decidir(ctx context.Context, m metaTransacao, desejado decisao) (certificado, error) {
	liberar := travarOperacao(&d.propostas, m.ID)
	defer liberar()
	for tentativa := 0; tentativa < 5; tentativa++ {
		d.mu.Lock()
		d.relogio++
		b := cedula{d.relogio, d.id}
		d.mu.Unlock()
		if err := d.g.SalvarBackup(); err != nil {
			return certificado{}, err
		}
		type resposta struct {
			ID  string
			P   respostaConsenso
			Err error
		}
		consultar := func(acao string, v *decisao) []resposta {
			rodada, cancelar := context.WithCancel(ctx)
			var chamadas sync.WaitGroup
			defer func() {
				cancelar()
				chamadas.Wait()
			}()
			ch := make(chan resposta, 3)
			for _, id := range d.membros {
				chamadas.Add(1)
				go func(id string) {
					defer chamadas.Done()
					var p respostaConsenso
					e := d.chamar(rodada, id, acao, mensagemConsenso{m, b, v}, &p)
					ch <- resposta{id, p, e}
				}(id)
			}
			rs := make([]resposta, 0, 3)
			validas := 0
			for range d.membros {
				r := <-ch
				rs = append(rs, r)
				if r.Err == nil {
					if r.P.Final != nil {
						return rs
					}
					if acao == "prometer" && r.P.Promessa == b {
						validas++
					}
					if acao == "aceitar" && r.P.Aceita == b && hmac.Equal([]byte(r.P.Voto), []byte(d.voto(m, b, *v, r.ID))) {
						validas++
					}
				}
				if validas >= 2 {
					return rs
				}
			}
			return rs
		}
		quantidade := 0
		v := desejado
		var maior cedula
		for _, r := range consultar("prometer", nil) {
			if r.Err != nil {
				continue
			}
			if r.P.Final != nil {
				return *r.P.Final, nil
			}
			d.mu.Lock()
			if r.P.Promessa.Numero > d.relogio {
				d.relogio = r.P.Promessa.Numero
			}
			d.mu.Unlock()
			if r.P.Promessa != b {
				continue
			}
			quantidade++
			if r.P.Valor != nil && r.P.Aceita.maior(maior) {
				maior = r.P.Aceita
				v = *r.P.Valor
			}
		}
		if quantidade < 2 {
			if ctx.Err() != nil {
				return certificado{}, ctx.Err()
			}
			continue
		}
		votos := make(map[string]string)
		for _, r := range consultar("aceitar", &v) {
			if r.Err == nil && r.P.Aceita == b && hmac.Equal([]byte(r.P.Voto), []byte(d.voto(m, b, v, r.ID))) {
				votos[r.ID] = r.P.Voto
			}
		}
		if len(votos) >= 2 {
			if d.aposQuorum != nil {
				if err := d.aposQuorum(); err != nil {
					return certificado{}, err
				}
			}
			return certificado{m, b, v, votos}, nil
		}
		select {
		case <-ctx.Done():
			return certificado{}, ctx.Err()
		case <-time.After(time.Duration(tentativa+1) * 15 * time.Millisecond):
		}
	}
	return certificado{}, fmt.Errorf("decisão pendente: maioria indisponível")
}
