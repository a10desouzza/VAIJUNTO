package servidor

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"vaijunto/internal/protocolo"
)

func cidade(s string) string      { return strings.Join(strings.Fields(s), " ") }
func chaveCidade(s string) string { return strings.ToLower(cidade(s)) }
func centavos(p float64) int64    { return int64(math.Round(p * 100)) }

func validarChave(s string) error {
	if len(strings.TrimSpace(s)) < 1 || len(s) > 100 {
		return fmt.Errorf("chave deve ter de 1 a 100 bytes")
	}
	return nil
}

func assinatura(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func (g *GrafoItinerarios) Publicar(token string, p protocolo.PublicacaoCarona) (protocolo.Carona, error) {
	if err := validarChave(p.Chave); err != nil {
		return protocolo.Carona{}, err
	}
	if len(p.Rota) < 2 || len(p.Rota) > 21 || len(p.Trechos) != len(p.Rota)-1 || p.Assentos < 1 || p.Assentos > 100 {
		return protocolo.Carona{}, fmt.Errorf("informe de 2 a 21 cidades, uma oferta por trecho e de 1 a 100 assentos")
	}
	p.Rota = append([]string(nil), p.Rota...)
	p.Trechos = append([]protocolo.OfertaTrecho(nil), p.Trechos...)
	if !decimalValido(p.ValorKM, 1000000) {
		return protocolo.Carona{}, fmt.Errorf("valor/km deve estar entre 0 e 1000000 e ter até duas casas decimais")
	}
	cidades := make(map[string]bool)
	for i, nome := range p.Rota {
		nome = cidade(nome)
		if err := protocolo.ValidarCidade(nome); err != nil {
			return protocolo.Carona{}, err
		}
		chave := chaveCidade(nome)
		if nome == "" || len(nome) > 100 || cidades[chave] {
			return protocolo.Carona{}, fmt.Errorf("cidades vazias, repetidas ou maiores que 100 bytes")
		}
		cidades[chave] = true
		p.Rota[i] = nome
	}
	horario, err := time.Parse(time.RFC3339, p.DataHora)
	if err != nil {
		return protocolo.Carona{}, fmt.Errorf("data_hora deve usar RFC3339 com fuso")
	}
	for _, t := range p.Trechos {
		if !decimalValido(t.DistanciaKM, 100000) || t.DistanciaKM <= 0 || p.ValorKM*t.DistanciaKM > 1000000 || t.TempoViagem < 1 || t.TempoViagem > 10080 || t.TempoParada < 0 || t.TempoParada > 10080 {
			return protocolo.Carona{}, fmt.Errorf("distância deve ser positiva, até 100000 km e duas casas decimais; preço calculado até 1000000; viagem de 1 a 10080 e parada de 0 a 10080 minutos")
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	u, err := g.autorizar(token, protocolo.Motorista)
	if err != nil {
		return protocolo.Carona{}, err
	}
	chave := u.Email + "|publicar|" + p.Chave
	sig := assinatura(p)
	if anterior, ok := g.chaves[chave]; ok {
		if anterior.assinatura != sig {
			return protocolo.Carona{}, fmt.Errorf("chave já utilizada com outros dados")
		}
		return g.consultarCarona(anterior.id), nil
	}
	if !horario.After(g.agora()) {
		return protocolo.Carona{}, fmt.Errorf("a partida deve estar no futuro")
	}
	id := g.novoID("C")
	o := &oferta{entrada: p, motorista: u.Email, status: protocolo.Ativa, ids: make([]string, 0, len(p.Trechos))}
	for i, ofertaTrecho := range p.Trechos {
		tid := g.novoID("T")
		precoCentavos := (centavos(p.ValorKM)*centavos(ofertaTrecho.DistanciaKM) + 50) / 100
		t := protocolo.Trecho{ID: tid, CaronaID: id, Origem: p.Rota[i], Destino: p.Rota[i+1], DataHora: horario.Format(time.RFC3339), Assentos: p.Assentos, Capacidade: p.Assentos, Preco: float64(precoCentavos) / 100, TempoViagem: ofertaTrecho.TempoViagem, TempoParada: ofertaTrecho.TempoParada, Motorista: u.Email, DistanciaKM: ofertaTrecho.DistanciaKM, Status: protocolo.Ativa}
		g.trechos[tid] = t
		origem := chaveCidade(t.Origem)
		g.rotas[origem] = append(g.rotas[origem], tid)
		g.ocupados[tid] = make(map[int]string)
		o.ids = append(o.ids, tid)
		horario = horario.Add(time.Duration(t.TempoViagem+t.TempoParada) * time.Minute)
	}
	g.caronas[id] = o
	g.chaves[chave] = repeticao{assinatura: sig, id: id}
	return g.consultarCarona(id), nil
}

func (g *GrafoItinerarios) consultarCarona(id string) protocolo.Carona {
	o := g.caronas[id]
	c := protocolo.Carona{ID: id, Motorista: o.motorista, Rota: append([]string(nil), o.entrada.Rota...), DataHora: o.entrada.DataHora, Status: o.status, Trechos: make([]protocolo.TrechoConsultado, 0, len(o.ids)), ValorKM: o.entrada.ValorKM}
	for _, tid := range o.ids {
		t := protocolo.TrechoConsultado{Trecho: g.trechos[tid], Passageiros: make([]protocolo.PassageiroConfirmado, 0)}
		for numero, rid := range g.ocupados[tid] {
			r := g.reservas[rid]
			t.Passageiros = append(t.Passageiros, protocolo.PassageiroConfirmado{ReservaID: rid, Passageiro: g.usuarios[r.Passageiro].usuario, Assento: numero})
		}
		sort.Slice(t.Passageiros, func(i, j int) bool { return t.Passageiros[i].Assento < t.Passageiros[j].Assento })
		c.Trechos = append(c.Trechos, t)
	}
	return c
}

func (g *GrafoItinerarios) ConsultarCaronas(token string) ([]protocolo.Carona, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	u, err := g.autorizar(token, protocolo.Motorista)
	if err != nil {
		return nil, err
	}
	resultado := make([]protocolo.Carona, 0)
	for id, o := range g.caronas {
		if o.motorista == u.Email {
			resultado = append(resultado, g.consultarCarona(id))
		}
	}
	sort.Slice(resultado, func(i, j int) bool { return resultado[i].ID < resultado[j].ID })
	return resultado, nil
}

func partida(t protocolo.Trecho) time.Time {
	h, _ := time.Parse(time.RFC3339, t.DataHora)
	return h
}

func chegada(t protocolo.Trecho) time.Time {
	return partida(t).Add(time.Duration(t.TempoViagem) * time.Minute)
}

func conectam(a, b protocolo.Trecho) bool {
	return chaveCidade(a.Destino) == chaveCidade(b.Origem) && !partida(b).Before(chegada(a).Add(time.Duration(a.TempoParada)*time.Minute))
}

func (g *GrafoItinerarios) Buscar(token string, b protocolo.BuscaItinerario) (protocolo.ResultadoBusca, error) {
	if chaveCidade(b.Origem) == "" || chaveCidade(b.Destino) == "" || chaveCidade(b.Origem) == chaveCidade(b.Destino) {
		return protocolo.ResultadoBusca{}, fmt.Errorf("origem e destino devem ser distintos e não vazios")
	}
	if _, err := time.Parse("2006-01-02", b.Data); err != nil {
		return protocolo.ResultadoBusca{}, fmt.Errorf("data obrigatória no formato AAAA-MM-DD")
	}
	if b.MaxTrechos == 0 {
		b.MaxTrechos = 12
	}
	if b.MaxTrechos < 1 || b.MaxTrechos > 20 {
		return protocolo.ResultadoBusca{}, fmt.Errorf("max_trechos deve estar entre 1 e 20")
	}
	if b.OrdenarPor == "" {
		b.OrdenarPor = "PRECO"
	}
	if b.OrdenarPor != "PRECO" && b.OrdenarPor != "TEMPO" && b.OrdenarPor != "TRECHOS" {
		return protocolo.ResultadoBusca{}, fmt.Errorf("ordenar_por deve ser PRECO, TEMPO ou TRECHOS")
	}
	g.mu.RLock()
	if _, err := g.autorizar(token, protocolo.Passageiro); err != nil {
		g.mu.RUnlock()
		return protocolo.ResultadoBusca{}, err
	}
	rotas := make(map[string][]protocolo.Trecho)
	agora := g.agora()
	for origem, ids := range g.rotas {
		for _, id := range ids {
			t := g.trechos[id]
			if g.disponivel(t, agora) {
				rotas[origem] = append(rotas[origem], t)
			}
		}
	}
	g.mu.RUnlock()
	resultado := protocolo.ResultadoBusca{Itinerarios: make([]protocolo.Itinerario, 0), MaxTrechos: b.MaxTrechos}
	visitadas := map[string]bool{chaveCidade(b.Origem): true}
	passos := 0
	var explorar func(string, []protocolo.Trecho, int64)
	explorar = func(origem string, caminho []protocolo.Trecho, preco int64) {
		for _, t := range rotas[origem] {
			if passos >= 50000 || len(resultado.Itinerarios) >= 100 {
				resultado.Limitada = true
				return
			}
			passos++
			destino := chaveCidade(t.Destino)
			if visitadas[destino] {
				continue
			}
			if len(caminho) == 0 {
				if partida(t).Format("2006-01-02") != b.Data {
					continue
				}
			} else if !conectam(caminho[len(caminho)-1], t) {
				continue
			}
			novo := append(caminho, t)
			total := preco + centavos(t.Preco)
			if destino == chaveCidade(b.Destino) {
				resultado.Itinerarios = append(resultado.Itinerarios, protocolo.Itinerario{Trechos: append([]protocolo.Trecho(nil), novo...), PrecoTotal: float64(total) / 100, DuracaoTotal: int(chegada(t).Sub(partida(novo[0])).Minutes())})
				continue
			}
			if len(novo) >= b.MaxTrechos {
				if len(rotas[destino]) > 0 {
					resultado.Limitada = true
				}
				continue
			}
			visitadas[destino] = true
			explorar(destino, novo, total)
			delete(visitadas, destino)
		}
	}
	explorar(chaveCidade(b.Origem), nil, 0)
	sort.Slice(resultado.Itinerarios, func(i, j int) bool {
		a, c := resultado.Itinerarios[i], resultado.Itinerarios[j]
		if b.OrdenarPor == "TEMPO" && a.DuracaoTotal != c.DuracaoTotal {
			return a.DuracaoTotal < c.DuracaoTotal
		}
		if b.OrdenarPor == "TRECHOS" && len(a.Trechos) != len(c.Trechos) {
			return len(a.Trechos) < len(c.Trechos)
		}
		if centavos(a.PrecoTotal) != centavos(c.PrecoTotal) {
			return centavos(a.PrecoTotal) < centavos(c.PrecoTotal)
		}
		if a.DuracaoTotal != c.DuracaoTotal {
			return a.DuracaoTotal < c.DuracaoTotal
		}
		if len(a.Trechos) != len(c.Trechos) {
			return len(a.Trechos) < len(c.Trechos)
		}
		for k := range a.Trechos {
			if a.Trechos[k].ID != c.Trechos[k].ID {
				return a.Trechos[k].ID < c.Trechos[k].ID
			}
		}
		return false
	})
	return resultado, nil
}
