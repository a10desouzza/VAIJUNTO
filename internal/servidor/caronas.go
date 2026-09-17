// internal/servidor/caronas.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Publicacao e busca no multigrafo. Cada oferta tem ID, horario, preco e vagas independentes.

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

// cidade: remove espacos excedentes do nome sem retirar acentos. Retorna: Nome sem espacos
// excedentes, preservando acentos e maiusculas.
func cidade(s string) string { return strings.Join(strings.Fields(s), " ") }

// chaveCidade: gera a chave de comparacao em minusculas. Retorna: Nome normalizado em minusculas,
// utilizado como chave dos mapas.
func chaveCidade(s string) string { return strings.ToLower(cidade(s)) }

// centavos: arredonda um valor em reais para centavos inteiros. Retorna: int64 com o valor
// arredondado em centavos.
func centavos(p float64) int64 { return int64(math.Round(p * 100)) }

// validarChave: confere preenchimento e tamanho da chave usada para identificar repeticoes.
// Retorna: nil se preenchida e com ate 100 bytes; error caso contrario.
func validarChave(s string) error {
	if len(strings.TrimSpace(s)) < 1 || len(s) > 100 {
		return fmt.Errorf("chave deve ter de 1 a 100 bytes")
	}
	return nil
}

// assinatura: serializa os dados para comparar repeticoes. Nao e uma assinatura criptografica.
// Retorna: JSON em string para comparar conteudos de tentativas; nao e um hash criptografico.
func assinatura(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// Publicar: valida a oferta e cria a carona e seus trechos. Retorna a carona publicada ou erro.
// Retorna: Carona criada ou anteriormente criada pela mesma chave e dados; error nas validacoes.
func (g *GrafoItinerarios) Publicar(sessaoID string, p protocolo.PublicacaoCarona) (protocolo.Carona, error) {
	if err := validarChave(p.Chave); err != nil {
		return protocolo.Carona{}, err
	}
	if len(p.Rota) < 2 || len(p.Rota) > 21 || len(p.Trechos) != len(p.Rota)-1 || p.Assentos < 1 || p.Assentos > 100 {
		return protocolo.Carona{}, fmt.Errorf("informe de 2 a 21 cidades, uma oferta por trecho e de 1 a 100 assentos")
	}
	/* Copiamos as listas recebidas para nao guardar vetores que o chamador possa alterar. */
	p.Rota = append([]string(nil), p.Rota...)
	p.Trechos = append([]protocolo.OfertaTrecho(nil), p.Trechos...)
	cidades := make(map[string]bool)
	for i, nome := range p.Rota {
		nome = cidade(nome)
		if err := protocolo.ValidarCidade(nome); err != nil {
			return protocolo.Carona{}, err
		}
		chave := chaveCidade(nome)
		if cidades[chave] {
			return protocolo.Carona{}, fmt.Errorf("cidades repetidas")
		}
		cidades[chave] = true
		p.Rota[i] = nome
	}
	horario, err := time.Parse(time.RFC3339, p.DataHora)
	if err != nil {
		return protocolo.Carona{}, fmt.Errorf("data_hora deve usar RFC3339 com fuso")
	}
	for _, t := range p.Trechos {
		if !decimalValido(t.DistanciaKM, 100000) || t.DistanciaKM <= 0 || !decimalValido(t.Preco, 1000000) || t.TempoViagem < 1 || t.TempoViagem > 10080 || t.TempoParada < 0 || t.TempoParada > 10080 {
			return protocolo.Carona{}, fmt.Errorf("distância deve ser positiva, até 100000 km e duas casas decimais; preço entre 0 e 1000000 com até duas casas decimais; viagem de 1 a 10080 e parada de 0 a 10080 minutos")
		}
	}
	// O destino final encerra a carona. Normalizamos antes da assinatura para
	// aceitar clientes antigos sem criar espera artificial nem alterar a entrada do chamador.
	p.Trechos[len(p.Trechos)-1].TempoParada = 0
	g.mu.Lock()
	defer g.mu.Unlock()
	u, err := g.autorizar(sessaoID, protocolo.Motorista)
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

		t := protocolo.Trecho{ID: tid, CaronaID: id, Origem: p.Rota[i], Destino: p.Rota[i+1], DataHora: horario.Format(time.RFC3339), Assentos: p.Assentos, Capacidade: p.Assentos, Preco: float64(centavos(ofertaTrecho.Preco)) / 100, TempoViagem: ofertaTrecho.TempoViagem, TempoParada: ofertaTrecho.TempoParada, Motorista: u.Email, DistanciaKM: ofertaTrecho.DistanciaKM, Status: protocolo.Ativa}
		g.trechos[tid] = t
		origem := chaveCidade(t.Origem)
		/* Append preserva ofertas paralelas: publicar a mesma rota nao substitui a anterior. */
		g.rotas[origem] = append(g.rotas[origem], tid)
		g.ocupados[tid] = make(map[int]string)
		o.ids = append(o.ids, tid)
		horario = horario.Add(time.Duration(t.TempoViagem+t.TempoParada) * time.Minute)
	}
	g.caronas[id] = o
	g.chaves[chave] = repeticao{assinatura: sig, id: id}
	return g.consultarCarona(id), nil
}

// consultarCarona: monta a resposta com vagas e passageiros atuais. Exige trava de leitura ou
// escrita. Retorna: Carona com uma nova lista de trechos e passageiros, refletindo as vagas
// atuais.
func (g *GrafoItinerarios) consultarCarona(id string) protocolo.Carona {
	o := g.caronas[id]
	c := protocolo.Carona{ID: id, Motorista: o.motorista, Rota: append([]string(nil), o.entrada.Rota...), DataHora: o.entrada.DataHora, Status: o.status, Trechos: make([]protocolo.TrechoConsultado, 0, len(o.ids))}
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

// ConsultarCaronas: retorna apenas as ofertas do motorista autenticado, ordenadas por ID. Retorna:
// Lista das caronas desse motorista, inclusive canceladas; lista vazia se nao houver; error de
// autorizacao.
func (g *GrafoItinerarios) ConsultarCaronas(sessaoID string) ([]protocolo.Carona, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	u, err := g.autorizar(sessaoID, protocolo.Motorista)
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

// partida: converte o horario RFC3339 do trecho, validado na publicacao. Retorna: time.Time
// correspondente a partida do trecho.
func partida(t protocolo.Trecho) time.Time {
	h, _ := time.Parse(time.RFC3339, t.DataHora)
	return h
}

// chegada: soma o tempo de viagem a partida, sem incluir a parada posterior. Retorna: Horario de
// chegada, sem a parada posterior.
func chegada(t protocolo.Trecho) time.Time {
	return partida(t).Add(time.Duration(t.TempoViagem) * time.Minute)
}

// conectam: confere cidade e horario da conexao, considerando a parada do trecho anterior.
// Retorna: true se as cidades coincidem e b parte apos a chegada mais a parada de a; false caso
// contrario.
func conectam(a, b protocolo.Trecho) bool {
	return chaveCidade(a.Destino) == chaveCidade(b.Origem) && !partida(b).Before(chegada(a).Add(time.Duration(a.TempoParada)*time.Minute))
}

// Buscar: recebe origem, destino, data; ordena por partida. Explora caminhos por DFS e retorna
// opcoes viaveis. Retorna: ResultadoBusca com opcoes, limite e aviso de exploracao limitada, ou
// error de validacao/autorizacao.
func (g *GrafoItinerarios) Buscar(sessaoID string, b protocolo.BuscaItinerario) (protocolo.ResultadoBusca, error) {
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
	g.mu.RLock()
	if _, err := g.autorizar(sessaoID, protocolo.Passageiro); err != nil {
		g.mu.RUnlock()
		return protocolo.ResultadoBusca{}, err
	}
	/* Copia de consulta: o DFS trabalha fora da trava. As vagas podem mudar depois,
	 * por isso Confirmar precisa verificar a disponibilidade novamente. */
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
			/* Limites evitam explorar indefinidamente um grafo com muitas combinacoes. */
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
			/* Backtracking: libera a cidade para outros caminhos, sem permitir ciclo no caminho atual. */
			delete(visitadas, destino)
		}
	}
	explorar(chaveCidade(b.Origem), nil, 0)
	/* Ordena o conjunto encontrado. Se a busca foi limitada, outras opcoes podem existir. */
	sort.Slice(resultado.Itinerarios, func(i, j int) bool {
		a, c := resultado.Itinerarios[i], resultado.Itinerarios[j]
		if !partida(a.Trechos[0]).Equal(partida(c.Trechos[0])) {
			return partida(a.Trechos[0]).Before(partida(c.Trechos[0]))
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
