// Autor: Arthur de Andrade Souza.
// Matéria: TEC502 - TP01 - Concorrência e Conectividade - 2026.2.
// Declaração de não plágio: Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
// Funcionalidade: Combina ofertas do consórcio e organiza a validação e a preparação de reservas distribuídas.
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
// Recebe: sessão do passageiro e critérios.
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
// Recebe: sessão e seleção de trechos com chave.
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
	if gs[d.id] == "" {
		return protocolo.Reserva{}, fmt.Errorf("coordenador indisponivel")
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
	m := metaTransacao{Lamport: d.evento(0), ID: id, Origem: d.id, Inicio: r.InicioItinerario, Tipo: "RESERVAR", Participantes: participantes, Hashes: make(map[string]string), Geracoes: gs}
	pedidos := make(map[string]protocolo.PedidoPreparacao)
	for _, empresa := range participantes {
		q := protocolo.PedidoPreparacao{TransacaoID: id, EmpresaOrigem: d.id, PassageiroRef: r.PassageiroRef, InicioItinerario: r.InicioItinerario, Participantes: participantes, TrechosIDs: grupos[empresa]}
		pedidos[empresa] = q
		m.Hashes[empresa] = hashJSON(q)
	}
	d.mu.Lock()
	d.origens[id] = &operacaoOrigem{Meta: m, Assinatura: hashJSON(refs), Passageiro: u.Email, Reserva: r}
	d.registros[id] = &registroTransacao{Meta: m, Criado: time.Now()}
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
	final, err := d.decidir(ctx, m, valor)
	if err != nil {
		return d.reservaOrigem(id), nil
	}
	d.propagar(ctx, final)
	d.atualizarOrigem(ctx, id)
	return d.reservaOrigem(id), nil
}

// trechoDoCatalogo converte um trecho publico do catalogo para o modelo utilizado nas buscas e reservas.
// Recebe: oferta pública de uma empresa.
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
// Recebe: contexto, metadados e pedidos por empresa.
func (d *Distribuido) prepararParticipantes(ctx context.Context, m metaTransacao, pedidos map[string]protocolo.PedidoPreparacao) decisao {
	v := decisao{Estado: "CONFIRMAR", Recibos: make(map[string]string)}

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
