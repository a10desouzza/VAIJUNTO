// Arquivo: internal/servidor/itinerario_distribuido.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Combina catálogos e coordena reservas que envolvem trechos de diferentes empresas.
// Agrupa os trechos por dono, prepara os participantes em ordem e solicita a decisão.
// Mantém a visão do passageiro sobre reservas pendentes, confirmadas ou canceladas.
// Também coordena cancelamentos e informa quais participantes estão indisponíveis.

package servidor

import (
	"context"
	"fmt"
	"sort"
	"time"

	"vaijunto/internal/consorcio"
	"vaijunto/internal/protocolo"
)

// buscar combina ofertas publicas e informa quando alguma empresa nao respondeu.
// Recebe: sessao string, b protocolo.BuscaItinerario.
// Retorna: (protocolo.ResultadoBusca, error). Em falha, devolve error; em sucesso, erro nil.
func (d *Distribuido) buscar(sessao string, b protocolo.BuscaItinerario) (protocolo.ResultadoBusca, error) {
	d.g.mu.RLock()
	_, err := d.g.autorizar(sessao, protocolo.Passageiro)
	d.g.mu.RUnlock()
	if err != nil {
		return protocolo.ResultadoBusca{}, err
	}
	if chaveCidade(b.Origem) == "" || chaveCidade(b.Destino) == "" || chaveCidade(b.Origem) == chaveCidade(b.Destino) {
		return protocolo.ResultadoBusca{}, fmt.Errorf("origem e destino devem ser distintos")
	}
	if _, err := time.Parse("2006-01-02", b.Data); err != nil {
		return protocolo.ResultadoBusca{}, fmt.Errorf("data inválida")
	}
	if b.MaxTrechos == 0 {
		b.MaxTrechos = 12
	}
	if b.MaxTrechos < 1 || b.MaxTrechos > 20 {
		return protocolo.ResultadoBusca{}, fmt.Errorf("max_trechos deve estar entre 1 e 20")
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	c := d.catalogo.ConsultarTodos(ctx)
	rotas := make(map[string][]protocolo.Trecho)
	for _, t := range c.Trechos {
		p := trechoDoCatalogo(t)
		rotas[chaveCidade(p.Origem)] = append(rotas[chaveCidade(p.Origem)], p)
	}
	r := buscarRotas(b, rotas)
	r.Indisponiveis = c.Indisponiveis
	return r, nil
}

// reservar prepara os donos em ordem e coordena uma decisao para o itinerario inteiro.
// Recebe: sessao string, p protocolo.ReservaItinerario.
// Retorna: a reserva agregada, que pode estar ATIVA, PENDENTE ou cancelada, ou erro de validacao/execucao.
func (d *Distribuido) reservar(sessao string, p protocolo.ReservaItinerario) (protocolo.Reserva, error) {
	if err := validarChave(p.Chave); err != nil {
		return protocolo.Reserva{}, err
	}
	d.g.mu.RLock()
	u, err := d.g.autorizar(sessao, protocolo.Passageiro)
	d.g.mu.RUnlock()
	if err != nil {
		return protocolo.Reserva{}, err
	}
	if len(p.Trechos) > 0 && len(p.TrechosIDs) > 0 {
		return protocolo.Reserva{}, fmt.Errorf("use referências por empresa ou IDs locais, não ambos")
	}
	refs := append([]protocolo.ReferenciaTrecho(nil), p.Trechos...)
	if len(refs) == 0 {
		for _, id := range p.TrechosIDs {
			refs = append(refs, protocolo.ReferenciaTrecho{EmpresaID: d.id, TrechoID: id})
		}
	}
	if len(refs) < 1 || len(refs) > 20 {
		return protocolo.Reserva{}, fmt.Errorf("informe de 1 a 20 trechos")
	}
	id := d.geracao + "-" + hashJSON([]string{u.Email, p.Chave})[:32]
	liberar := travarOperacao(&d.chamadas, id)
	defer liberar()
	d.mu.Lock()
	anterior := d.origens[id]
	if anterior != nil {
		anterior = copiaJSON(anterior)
	}
	d.mu.Unlock()
	ctx, cancelar := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelar()
	if anterior != nil {
		if anterior.Assinatura != hashJSON(refs) {
			return protocolo.Reserva{}, fmt.Errorf("chave já utilizada com outros dados")
		}
		d.atualizarOrigem(ctx, id)
		return d.reservaOrigem(id), nil
	}
	// Uma viagem inteiramente local nao depende de testemunhas remotas.
	soLocal := true
	idsLocais := make([]string, 0, len(refs))
	for _, ref := range refs {
		soLocal = soLocal && ref.EmpresaID == d.id
		idsLocais = append(idsLocais, ref.TrechoID)
	}
	if soLocal {
		return d.g.Confirmar(sessao, protocolo.ReservaItinerario{Chave: p.Chave, TrechosIDs: idsLocais})
	}
	d.g.mu.RLock()
	_, chaveLocal := d.g.chaves[u.Email+"|reservar|"+p.Chave]
	d.g.mu.RUnlock()
	if chaveLocal {
		return protocolo.Reserva{}, fmt.Errorf("chave já utilizada com outros dados")
	}
	select {
	case d.limite <- struct{}{}:
		defer func() {
			<-d.limite
		}()
	default:
		return protocolo.Reserva{}, fmt.Errorf("servidor ocupado; tente novamente")
	}
	gs := d.geracoes(ctx)
	if len(gs) < 2 {
		return protocolo.Reserva{}, fmt.Errorf("duas empresas precisam estar disponíveis para decidir")
	}
	c := d.catalogo.ConsultarTodos(ctx)
	catalogo := make(map[protocolo.ReferenciaTrecho]protocolo.Trecho)
	for _, t := range c.Trechos {
		ref := protocolo.ReferenciaTrecho{EmpresaID: t.EmpresaID, TrechoID: t.ID}
		catalogo[ref] = trechoDoCatalogo(t)
	}
	grupos, err := consorcio.AgruparTrechos(protocolo.ReservaDistribuida{Chave: p.Chave, Trechos: refs})
	if err != nil {
		return protocolo.Reserva{}, err
	}
	vistos := make(map[protocolo.ReferenciaTrecho]bool)
	cidades := make(map[string]bool)
	r := protocolo.Reserva{ID: id, TransacaoID: id, EmpresaOrigem: d.id, Passageiro: u.Email, PassageiroRef: d.assinar([]string{d.id, u.Email}), Status: pendente, CriadaEm: time.Now().UTC().Format(time.RFC3339), Assentos: make([]protocolo.AssentoReservado, 0, len(refs))}
	var anteriorTrecho protocolo.Trecho
	var total int64
	for i, ref := range refs {
		t, ok := catalogo[ref]
		if !ok || vistos[ref] || !contem(d.membros, ref.EmpresaID) || gs[ref.EmpresaID] == "" {
			return protocolo.Reserva{}, fmt.Errorf("trecho inexistente, repetido ou empresa indisponível")
		}
		vistos[ref] = true
		if i == 0 {
			cidades[chaveCidade(t.Origem)] = true
			r.InicioItinerario = t.DataHora
		} else if !conectam(anteriorTrecho, t) {
			return protocolo.Reserva{}, fmt.Errorf("trechos sem conexão espacial ou temporal")
		}
		if cidades[chaveCidade(t.Destino)] {
			return protocolo.Reserva{}, fmt.Errorf("itinerário contém ciclo")
		}
		cidades[chaveCidade(t.Destino)] = true
		r.Assentos = append(r.Assentos, protocolo.AssentoReservado{Trecho: t})
		total += centavos(t.Preco)
		anteriorTrecho = t
	}
	r.PrecoTotal = float64(total) / 100
	participantes := make([]string, 0, len(grupos))
	for empresa := range grupos {
		participantes = append(participantes, empresa)
	}
	sort.Strings(participantes)
	m := metaTransacao{ID: id, Origem: d.id, Inicio: r.InicioItinerario, Tipo: "RESERVAR", Participantes: participantes, Hashes: make(map[string]string), Geracoes: gs}
	pedidos := make(map[string]protocolo.PedidoPreparacao)
	for _, empresa := range participantes {
		q := protocolo.PedidoPreparacao{TransacaoID: id, EmpresaOrigem: d.id, PassageiroRef: r.PassageiroRef, InicioItinerario: r.InicioItinerario, Participantes: participantes, TrechosIDs: grupos[empresa]}
		pedidos[empresa] = q
		m.Hashes[empresa] = hashJSON(q)
	}
	d.mu.Lock()
	d.origens[id] = &operacaoOrigem{Meta: m, Assinatura: hashJSON(refs), Passageiro: u.Email, Reserva: r}
	d.registros[id] = &registroConsenso{Meta: m, Criado: time.Now()}
	d.mu.Unlock()
	if err := d.g.SalvarBackup(); err != nil {
		return protocolo.Reserva{}, err
	}
	d.g.logger.Printf("RESERVA transacao=%s coordenador=%s fase=preparar participantes=%v", id, d.id, participantes)
	valor := d.prepararParticipantes(ctx, m, pedidos)
	if d.aposPreparar != nil {
		if err := d.aposPreparar(); err != nil {
			return d.reservaOrigem(id), err
		}
	}
	cert, err := d.decidir(ctx, m, valor)
	if err != nil {
		return d.reservaOrigem(id), nil
	}
	d.propagar(ctx, cert)
	d.atualizarOrigem(ctx, id)
	return d.reservaOrigem(id), nil
}

// trechoDoCatalogo converte um trecho publico do catalogo para o modelo utilizado nas buscas e reservas.
// Recebe: t consorcio.TrechoPublico.
// Retorna: protocolo.Trecho.
func trechoDoCatalogo(t consorcio.TrechoPublico) protocolo.Trecho {
	return protocolo.Trecho{
		EmpresaID: t.EmpresaID, ID: t.ID, CaronaID: t.CaronaID,
		Origem: t.Origem, Destino: t.Destino, DataHora: t.DataHora,
		Assentos: t.Assentos, Capacidade: t.Capacidade, Preco: t.Preco,
		DistanciaKM: t.DistanciaKM, TempoViagem: t.TempoViagem,
		TempoParada: t.TempoParada, Status: protocolo.Ativa,
	}
}

// prepararParticipantes prepara os donos em ordem de ID; ao primeiro conflito propoe aborto, sem esperar pela liberacao da vaga.
// Recebe: ctx context.Context, m metaTransacao, pedidos map[string]protocolo.PedidoPreparacao.
// Retorna: a proposta CONFIRMAR com recibos de todos os donos, ou ABORTAR ao primeiro erro. Ainda nao e a decisao final.
func (d *Distribuido) prepararParticipantes(ctx context.Context, m metaTransacao, pedidos map[string]protocolo.PedidoPreparacao) decisao {
	v := decisao{Estado: "CONFIRMAR", Recibos: make(map[string]string)}
	// Todos adquirem empresas na mesma ordem e o dono recusa conflitos sem
	// aguardar recursos: nenhuma transacao mantem uma espera circular remota.
	// Para a disputa do mesmo A+B, so quem preparar o primeiro dono avanca ao
	// segundo. A ordem de admissao no dono desempata as duas requisicoes.
	for _, empresa := range m.Participantes {
		var r resultadoRPC
		err := d.chamar(ctx, empresa, "preparar", prepararRPC{m, pedidos[empresa]}, &r)
		if err != nil {
			v.Estado = "ABORTAR"
			v.Motivo = err.Error()
			break
		}
		v.Recibos[empresa] = r.Recibo
	}
	return v
}

// reservaOrigem consulta uma copia da reserva agregada mantida pela empresa de origem.
// Recebe: id string.
// Retorna: uma copia da reserva agregada; uma reserva vazia quando o identificador nao pertence as origens locais.
func (d *Distribuido) reservaOrigem(id string) protocolo.Reserva {
	d.mu.Lock()
	defer d.mu.Unlock()
	if op := d.origens[id]; op != nil {
		return copiarReserva(op.Reserva)
	}
	return protocolo.Reserva{}
}

// atualizarOrigem consulta e propaga a decisao; marca a reserva ativa somente apos aplicacao em todos os donos.
// Recebe: ctx context.Context, id string.
// Retorna: nenhum valor; executa os efeitos descritos acima.
func (d *Distribuido) atualizarOrigem(ctx context.Context, id string) {
	d.mu.Lock()
	op := copiaJSON(d.origens[id])
	reg := copiaJSON(d.registros[id])
	d.mu.Unlock()
	if op == nil || op.Reserva.Status != pendente || reg == nil || reg.Final == nil {
		return
	}
	c := *reg.Final
	rs := d.propagar(ctx, c)
	r := op.Reserva
	if c.Valor.Estado == "ABORTAR" {
		r.Status = protocolo.Cancelada
		r.Motivo = c.Valor.Motivo
	} else {
		for _, empresa := range c.Meta.Participantes {
			if rs[empresa].Reserva.ID == "" {
				return
			}
		}
		for i, a := range r.Assentos {
			for _, local := range rs[a.Trecho.EmpresaID].Reserva.Assentos {
				if local.Trecho.ID == a.Trecho.ID {
					r.Assentos[i].Numero = local.Numero
					r.Assentos[i].Trecho.Assentos = local.Trecho.Assentos
				}
			}
		}
		r.Status = protocolo.Ativa
		r.Motivo = ""
	}
	d.mu.Lock()
	if atual := d.origens[id]; atual != nil && atual.Reserva.Status == pendente {
		atual.Reserva = r
	}
	d.mu.Unlock()
	if err := d.g.SalvarBackup(); err != nil {
		d.g.logger.Printf("BACKUP transacao=%s erro=%v", id, err)
	}
}

// atualizarOrigens atualiza as reservas pendentes dos passageiros da empresa de origem.
// Recebe: ctx context.Context.
// Retorna: nenhum valor; executa os efeitos descritos acima.
func (d *Distribuido) atualizarOrigens(ctx context.Context) {
	d.mu.Lock()
	ids := make([]string, 0)
	for id, op := range d.origens {
		if op.Reserva.Status == pendente {
			ids = append(ids, id)
		}
	}
	d.mu.Unlock()
	for _, id := range ids {
		d.atualizarOrigem(ctx, id)
	}
}

// consultar preserva a reserva e verifica a disponibilidade atual de seus participantes.
// Recebe: sessao string.
// Retorna: o resultado da consulta ou erro de acesso/validacao, conforme o recurso deste pacote.
func (d *Distribuido) consultar(sessao string) ([]protocolo.Reserva, error) {
	d.g.mu.RLock()
	u, err := d.g.autorizar(sessao, protocolo.Passageiro)
	d.g.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	locais, err := d.g.ConsultarReservas(sessao)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	for _, op := range d.origens {
		if op.Passageiro == u.Email {
			locais = append(locais, copiarReserva(op.Reserva))
		}
	}
	d.mu.Unlock()
	ctx, cancelar := context.WithTimeout(context.Background(), d.cliente.Timeout)
	defer cancelar()
	disponiveis := d.geracoes(ctx)
	for i, reserva := range locais {
		if reserva.Status == protocolo.Cancelada {
			continue
		}
		empresas := make(map[string]bool)
		for _, assento := range reserva.Assentos {
			id := assento.Trecho.EmpresaID
			if id != "" && disponiveis[id] == "" {
				empresas[id] = true
			}
		}
		for id := range empresas {
			locais[i].EmpresasIndisponiveis = append(locais[i].EmpresasIndisponiveis, id)
		}
		sort.Strings(locais[i].EmpresasIndisponiveis)
	}
	sort.Slice(locais, func(i, j int) bool {
		return locais[i].ID < locais[j].ID
	})
	return locais, nil
}

// cancelar autoriza o passageiro e encaminha o cancelamento para a reserva local ou para o protocolo distribuido.
// Recebe: sessao, id string.
// Retorna: (protocolo.Reserva, error). Em falha, devolve error; em sucesso, erro nil.
func (d *Distribuido) cancelar(sessao, id string) (protocolo.Reserva, error) {
	d.g.mu.RLock()
	u, err := d.g.autorizar(sessao, protocolo.Passageiro)
	d.g.mu.RUnlock()
	if err != nil {
		return protocolo.Reserva{}, err
	}
	d.mu.Lock()
	op := copiaJSON(d.origens[id])
	d.mu.Unlock()
	if op == nil {
		return d.g.CancelarReserva(sessao, id)
	}
	if op.Passageiro != u.Email {
		return protocolo.Reserva{}, fmt.Errorf("reserva não encontrada para este passageiro")
	}
	if err := d.cancelarGlobal(context.Background(), op.Meta, "Cancelada pelo passageiro."); err != nil {
		return protocolo.Reserva{}, err
	}
	return d.reservaOrigem(id), nil
}

// cancelarGlobal usa uma nova decisao conjunta antes de devolver os assentos.
// Recebe: ctx context.Context, original metaTransacao, motivo string.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
func (d *Distribuido) cancelarGlobal(ctx context.Context, original metaTransacao, motivo string) error {
	liberar := travarOperacao(&d.chamadas, original.ID+":cancelar")
	defer liberar()
	ctx, cancelar := context.WithTimeout(ctx, 20*time.Second)
	defer cancelar()
	m := copiaJSON(original)
	m.ID = original.ID + ":cancelar"
	m.Tipo = "CANCELAR"
	m.Original = original.ID
	pedidos := make(map[string]protocolo.PedidoPreparacao)
	m.Hashes = make(map[string]string)
	for _, empresa := range m.Participantes {
		q := protocolo.PedidoPreparacao{TransacaoID: m.ID, EmpresaOrigem: m.Origem, PassageiroRef: "cancelamento", InicioItinerario: m.Inicio, Participantes: m.Participantes, TrechosIDs: []string{"cancelamento"}}
		pedidos[empresa] = q
		m.Hashes[empresa] = hashJSON(q)
	}
	d.mu.Lock()
	existente := copiaJSON(d.registros[m.ID])
	if existente != nil && existente.Final != nil && existente.Final.Valor.Estado == "ABORTAR" {
		// Uma recusa anterior nao impede tentar um novo cancelamento depois.
		d.relogio++
		m.ID = original.ID + ":c:" + hashJSON([]any{d.geracao, d.relogio})[:16]
		existente = nil
		for empresa, q := range pedidos {
			q.TransacaoID = m.ID
			pedidos[empresa] = q
			m.Hashes[empresa] = hashJSON(q)
		}
	}
	d.mu.Unlock()
	if err := d.g.SalvarBackup(); err != nil {
		return err
	}
	var cert certificado
	var err error
	if existente != nil && existente.Final != nil {
		cert = *existente.Final
	} else {
		v := d.prepararParticipantes(ctx, m, pedidos)
		if v.Estado == "CONFIRMAR" {
			v.Motivo = motivo
		}
		cert, err = d.decidir(ctx, m, v)
		if err != nil {
			return err
		}
	}
	rs := d.propagar(ctx, cert)
	if cert.Valor.Estado != "CONFIRMAR" {
		return fmt.Errorf("cancelamento recusado: %s", cert.Valor.Motivo)
	}
	for _, id := range m.Participantes {
		if _, ok := rs[id]; !ok {
			return fmt.Errorf("cancelamento decidido, aplicação pendente em %s; consulte novamente", id)
		}
	}
	return nil
}

// cancelarOferta resolve as reservas distribuidas antes de alterar a carona do motorista.
// Recebe: sessao, id string, trecho bool.
// Retorna: (protocolo.Carona, error). Em falha, devolve error; em sucesso, erro nil.
func (d *Distribuido) cancelarOferta(sessao, id string, trecho bool) (protocolo.Carona, error) {
	d.g.mu.RLock()
	u, err := d.g.autorizar(sessao, protocolo.Motorista)
	var ids []string
	if err == nil {
		if trecho {
			e := d.g.trechos[id]
			if e == nil || e.trecho.Motorista != u.Email {
				err = fmt.Errorf("trecho não encontrado")
			} else {
				ids = []string{id}
			}
		} else {
			o := d.g.caronas[id]
			if o == nil || o.motorista != u.Email {
				err = fmt.Errorf("carona não encontrada")
			} else {
				ids = append([]string(nil), o.ids...)
			}
		}
	}
	transacoes := make(map[string]bool)
	if err == nil {
		for _, tid := range ids {
			if len(d.g.trechos[tid].preparados) > 0 {
				err = fmt.Errorf("há uma preparação pendente; tente novamente após a decisão")
				break
			}
			for _, rid := range d.g.trechos[tid].ocupados {
				if tx := d.g.reservas[rid].TransacaoID; tx != "" {
					transacoes[tx] = true
				}
			}
		}
	}
	d.g.mu.RUnlock()
	if err != nil {
		return protocolo.Carona{}, err
	}
	for tx := range transacoes {
		d.mu.Lock()
		r := copiaJSON(d.registros[tx])
		d.mu.Unlock()
		if r == nil {
			return protocolo.Carona{}, fmt.Errorf("metadados da reserva indisponíveis")
		}
		if err := d.cancelarGlobal(context.Background(), r.Meta, "Oferta cancelada pelo motorista."); err != nil {
			return protocolo.Carona{}, err
		}
	}
	if trecho {
		return d.g.CancelarTrecho(sessao, id)
	}
	return d.g.CancelarCarona(sessao, id)
}
