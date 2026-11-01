// Arquivo: internal/carga/carga.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Executa os cenários de carga local e entre as três empresas usando clientes TCP.
// Mede respostas, latências e vazão e confere a ocupação dos assentos nos donos.
// Centraliza as métricas e a limpeza das ofertas criadas pela simulação.

package carga

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"vaijunto/internal/cliente"
	"vaijunto/internal/protocolo"
)

// Metricas resume o resultado da simulacao.
type Metricas struct {
	Empresas           int     `json:"empresas,omitempty"`
	Buscas             int     `json:"buscas,omitempty"`
	Pendentes          int     `json:"pendentes,omitempty"`
	Clientes           int     `json:"clientes"`
	VagasPorTrecho     int     `json:"vagas_por_trecho"`
	Confirmadas        int     `json:"confirmadas"`
	Recusadas          int     `json:"recusadas"`
	ErrosTransporte    int     `json:"erros_transporte"`
	DuracaoMS          float64 `json:"duracao_ms"`
	LatenciaMediaMS    float64 `json:"latencia_media_ms"`
	LatenciaP95MS      float64 `json:"latencia_p95_ms"`
	RequisicoesSegundo float64 `json:"requisicoes_por_segundo"`
	Integridade        bool    `json:"integridade"`
}

// chamar envia a operacao TCP e converte os dados para alvo. Propaga falhas como erro.
// Recebe: endereco, acao, sessaoID string, dados any, alvo any.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
func chamar(endereco, acao, sessaoID string, dados any, alvo any) error {
	raw, err := json.Marshal(dados)
	if err != nil {
		return err
	}
	resposta, err := cliente.EnviarRequisicao(endereco, protocolo.Requisicao{Acao: acao, SessaoID: sessaoID, Dados: raw})
	if err != nil {
		return err
	}
	if resposta.Status != protocolo.Sucesso {
		return fmt.Errorf("%s: %s", acao, resposta.Mensagem)
	}
	if alvo == nil {
		return nil
	}
	raw, err = json.Marshal(resposta.Dados)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, alvo)
}

// criarUsuario cadastra uma conta de teste e recebe a sessao criada automaticamente.
// Recebe: endereco, email, perfil string.
// Retorna: (string, error). Em falha, devolve error; em sucesso, erro nil.
func criarUsuario(endereco, email, perfil string) (string, error) {
	senha := "senha-carga-" + email
	var sessao protocolo.Sessao
	err := chamar(endereco, protocolo.AcaoCadastrar, "", protocolo.Cadastro{Nome: "Teste de carga", Email: email, Senha: senha, Perfil: perfil}, &sessao)
	return sessao.ID, err
}

// Executar Cadastra dois motoristas, publica dois trechos conectados e autentica os passageiros.
// concluir a fase de uso, tenta cancelar as ofertas e encerrar as sessoes dos passageiros.
// Recebe: endereco string, quantidade, vagas int.
// Retorna: (Metricas, error). Em falha, devolve error; em sucesso, erro nil.
func Executar(endereco string, quantidade, vagas int) (Metricas, error) {
	m := Metricas{Clientes: quantidade, VagasPorTrecho: vagas}
	if quantidade < 1 || quantidade > 200 || vagas < 1 || vagas > 100 {
		return m, fmt.Errorf("clientes deve estar entre 1 e 200; vagas entre 1 e 100")
	}
	aleatorio := make([]byte, 8)
	if _, err := rand.Read(aleatorio); err != nil {
		return m, err
	}
	prefixo := hex.EncodeToString(aleatorio)
	motoristas := make([]string, 2)
	caronas := make([]protocolo.Carona, 2)

	for i := range motoristas {
		sessaoID, err := criarUsuario(endereco, fmt.Sprintf("m%d-%s@carga.test", i, prefixo), protocolo.Motorista)
		if err != nil {
			return m, err
		}
		motoristas[i] = sessaoID
		cidades := []string{"Carga-" + prefixo + "-A", "Carga-" + prefixo + "-B", "Carga-" + prefixo + "-C"}
		hora := "2099-10-01T08:00:00-03:00"
		if i == 1 {
			hora = "2099-10-01T09:15:00-03:00"
		}
		oferta := protocolo.PublicacaoCarona{Chave: "carga", Rota: cidades[i : i+2], DataHora: hora, Assentos: vagas, Trechos: []protocolo.OfertaTrecho{{Preco: 10, DistanciaKM: 10, TempoViagem: 60}}}
		if err := chamar(endereco, protocolo.AcaoPublicar, sessaoID, oferta, &caronas[i]); err != nil {
			return m, err
		}
	}
	defer func() {
		for i, sessaoID := range motoristas {
			chamar(endereco, protocolo.AcaoCancelarCarona, sessaoID, protocolo.Identificador{ID: caronas[i].ID}, nil)
		}
	}()
	sessoes := make([]string, quantidade)
	for i := range sessoes {
		sessaoID, err := criarUsuario(endereco, fmt.Sprintf("p%d-%s@carga.test", i, prefixo), protocolo.Passageiro)
		if err != nil {
			return m, err
		}
		sessoes[i] = sessaoID
	}
	defer func() {
		for _, sessaoID := range sessoes {
			chamar(endereco, protocolo.AcaoSair, sessaoID, struct{}{}, nil)
		}
	}()
	ids := []string{caronas[0].Trechos[0].Trecho.ID, caronas[1].Trechos[0].Trecho.ID}
	pedido, _ := json.Marshal(protocolo.ReservaItinerario{Chave: "unica", TrechosIDs: ids})
	type resultado struct {
		resposta *protocolo.Resposta
		err      error
		duracao  time.Duration
	}
	resultados := make([]resultado, quantidade)
	/* Barreira de inicio: todas as goroutines esperam o fechamento do mesmo canal. */
	inicio := make(chan struct{})
	var wg sync.WaitGroup
	for i, sessaoID := range sessoes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-inicio
			/* A medicao comeca apos preparar usuarios e ofertas; cobre a disputa pelas reservas. */
			instante := time.Now()
			resposta, err := cliente.EnviarRequisicao(endereco, protocolo.Requisicao{Acao: protocolo.AcaoConfirmar, SessaoID: sessaoID, Dados: pedido})
			/* Cada goroutine escreve em seu indice; a leitura so ocorre depois do Wait. */
			resultados[i] = resultado{resposta: resposta, err: err, duracao: time.Since(instante)}
		}()
	}
	instante := time.Now()
	close(inicio)
	wg.Wait()
	duracao := time.Since(instante)
	latencias := make([]float64, quantidade)
	confirmadas := make(map[string]bool)
	integridade := true
	for i, r := range resultados {
		latencias[i] = float64(r.duracao) / float64(time.Millisecond)
		if r.err != nil {
			m.ErrosTransporte++
			continue
		}
		if r.resposta.Status == protocolo.Erro {
			m.Recusadas++
			continue
		}
		m.Confirmadas++
		var reserva protocolo.Reserva
		raw, _ := json.Marshal(r.resposta.Dados)
		if err := json.Unmarshal(raw, &reserva); err != nil {
			integridade = false
			continue
		}
		if confirmadas[reserva.ID] || len(reserva.Assentos) != 2 || reserva.Status != protocolo.Ativa {
			integridade = false
		}
		confirmadas[reserva.ID] = true
		for j, a := range reserva.Assentos {
			if j >= len(ids) || a.Trecho.ID != ids[j] {
				integridade = false
			}
		}
	}
	m.calcularTempos(latencias, duracao)
	for i, sessaoID := range motoristas {
		var publicadas []protocolo.Carona
		if err := chamar(endereco, protocolo.AcaoCaronas, sessaoID, struct{}{}, &publicadas); err != nil {
			return m, err
		}
		if len(publicadas) != 1 || len(publicadas[0].Trechos) != 1 {
			return m, fmt.Errorf("carona de carga ausente")
		}
		t := publicadas[0].Trechos[0]
		if t.Trecho.ID != ids[i] || t.Trecho.Assentos != vagas-m.Confirmadas || len(t.Passageiros) != m.Confirmadas {
			integridade = false
		}
		assentos := make(map[int]bool)
		for _, p := range t.Passageiros {
			if assentos[p.Assento] || p.Assento < 1 || p.Assento > vagas || !confirmadas[p.ReservaID] {
				integridade = false
			}
			assentos[p.Assento] = true
		}
	}
	esperado := min(vagas, quantidade)
	m.Integridade = integridade && m.ErrosTransporte == 0 && m.Confirmadas == esperado && m.Recusadas == quantidade-esperado
	if !m.Integridade {
		return m, fmt.Errorf("teste de carga falhou na integridade ou no transporte")
	}
	return m, nil
}

// ExecutarConsorcio gera buscas e reservas a partir das tres empresas. Confere
// os assentos locais contra as transacoes globais, e nao apenas respostas HTTP.
// Recebe: enderecos []string, quantidade, vagas int.
// Retorna: as metricas da simulacao e eventual erro de configuracao, transporte ou integridade.
func ExecutarConsorcio(enderecos []string, quantidade, vagas int) (Metricas, error) {
	m := Metricas{Empresas: len(enderecos), Clientes: quantidade, VagasPorTrecho: vagas}
	if len(enderecos) != 3 || quantidade < 1 || quantidade > 200 || vagas < 1 || vagas > 100 {
		return m, fmt.Errorf("informe três servidores, de 1 a 200 clientes e de 1 a 100 vagas")
	}
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return m, err
	}
	prefixo := hex.EncodeToString(b)
	cidades := []string{"Carga-" + prefixo + "-A", "Carga-" + prefixo + "-B", "Carga-" + prefixo + "-C", "Carga-" + prefixo + "-D"}
	motoristas := make([]string, 3)
	caronas := make([]protocolo.Carona, 3)
	for i, endereco := range enderecos {
		s, err := criarUsuario(endereco, fmt.Sprintf("m%d-%s@carga.test", i, prefixo), protocolo.Motorista)
		if err != nil {
			return m, err
		}
		motoristas[i] = s
		p := protocolo.PublicacaoCarona{Chave: "oferta", Rota: cidades[i : i+2], DataHora: fmt.Sprintf("2099-10-01T%02d:00:00-03:00", 8+i), Assentos: vagas, Trechos: []protocolo.OfertaTrecho{{Preco: 10, DistanciaKM: 10, TempoViagem: 30}}}
		if err := chamar(endereco, protocolo.AcaoPublicar, s, p, &caronas[i]); err != nil {
			return m, err
		}
	}
	type resultado struct {
		Reserva    protocolo.Reserva
		Transporte bool
		Err        error
		Duracao    time.Duration
		Buscou     bool
	}
	sessoes := make([]string, quantidade)
	rs := make([]resultado, quantidade)
	for i := range sessoes {
		s, err := criarUsuario(enderecos[i%3], fmt.Sprintf("p%d-%s@carga.test", i, prefixo), protocolo.Passageiro)
		if err != nil {
			return m, err
		}
		sessoes[i] = s
	}
	defer func() {
		for i, s := range sessoes {
			if rs[i].Reserva.Status == protocolo.Ativa {
				_ = chamar(enderecos[i%3], protocolo.AcaoCancelarReserva, s, protocolo.Identificador{ID: rs[i].Reserva.ID}, nil)
			}
			_ = chamar(enderecos[i%3], protocolo.AcaoSair, s, struct{}{}, nil)
		}
		for i, s := range motoristas {
			_ = chamar(enderecos[i], protocolo.AcaoCancelarCarona, s, protocolo.Identificador{ID: caronas[i].ID}, nil)
			_ = chamar(enderecos[i], protocolo.AcaoSair, s, struct{}{}, nil)
		}
	}()
	inicio := make(chan struct{})
	var wg sync.WaitGroup
	for i, s := range sessoes {
		wg.Add(1)
		go func(i int, s string) {
			defer wg.Done()
			<-inicio
			t := time.Now()
			r := resultado{}
			defer func() {
				r.Duracao = time.Since(t)
				rs[i] = r
			}()
			endereco := enderecos[i%3]
			var busca protocolo.ResultadoBusca
			if err := chamar(endereco, protocolo.AcaoBuscar, s, protocolo.BuscaItinerario{Origem: cidades[0], Destino: cidades[3], Data: "2099-10-01"}, &busca); err != nil {
				r.Err = err
				return
			}
			r.Buscou = true
			if len(busca.Itinerarios) == 0 {
				r.Err = fmt.Errorf("nenhum itinerário disponível")
				return
			}
			p := protocolo.ReservaItinerario{Chave: "unica"}
			for _, trecho := range busca.Itinerarios[0].Trechos {
				p.Trechos = append(p.Trechos, protocolo.ReferenciaTrecho{EmpresaID: trecho.EmpresaID, TrechoID: trecho.ID})
			}
			raw, _ := json.Marshal(p)
			resposta, err := cliente.EnviarRequisicao(endereco, protocolo.Requisicao{Acao: protocolo.AcaoConfirmar, SessaoID: s, Dados: raw})
			if err != nil {
				r.Err = err
				r.Transporte = true
				return
			}
			if resposta.Status != protocolo.Sucesso {
				r.Err = fmt.Errorf("%s", resposta.Mensagem)
				return
			}
			raw, _ = json.Marshal(resposta.Dados)
			if err := json.Unmarshal(raw, &r.Reserva); err != nil {
				r.Err = err
				return
			}
			limite := time.Now().Add(25 * time.Second)
			for r.Reserva.Status == "PENDENTE" && time.Now().Before(limite) {
				time.Sleep(100 * time.Millisecond)
				var reservas []protocolo.Reserva
				if err := chamar(endereco, protocolo.AcaoReservas, s, struct{}{}, &reservas); err != nil {
					r.Err = err
					return
				}
				for _, reserva := range reservas {
					if reserva.ID == r.Reserva.ID {
						r.Reserva = reserva
						break
					}
				}
			}
		}(i, s)
	}
	t := time.Now()
	close(inicio)
	wg.Wait()
	duracao := time.Since(t)
	latencias := make([]float64, quantidade)
	confirmadas := make(map[string]bool)
	integridade := true
	for i, r := range rs {
		latencias[i] = float64(r.Duracao) / float64(time.Millisecond)
		if r.Buscou {
			m.Buscas++
		}
		if r.Transporte {
			m.ErrosTransporte++
			continue
		}
		if r.Err != nil || r.Reserva.Status == protocolo.Cancelada {
			m.Recusadas++
			continue
		}
		if r.Reserva.Status == "PENDENTE" {
			m.Pendentes++
			continue
		}
		if r.Reserva.Status != protocolo.Ativa || len(r.Reserva.Assentos) != 3 || confirmadas[r.Reserva.ID] {
			integridade = false
			continue
		}
		m.Confirmadas++
		confirmadas[r.Reserva.ID] = true
	}
	for i, s := range motoristas {
		var publicadas []protocolo.Carona
		if err := chamar(enderecos[i], protocolo.AcaoCaronas, s, struct{}{}, &publicadas); err != nil {
			return m, err
		}
		encontrou := false
		for _, c := range publicadas {
			if c.ID != caronas[i].ID {
				continue
			}
			encontrou = true
			trecho := c.Trechos[0]
			if trecho.Trecho.Assentos != vagas-m.Confirmadas || len(trecho.Passageiros) != m.Confirmadas {
				integridade = false
			}
			vistos := make(map[int]bool)
			for _, p := range trecho.Passageiros {
				if vistos[p.Assento] || p.Assento < 1 || p.Assento > vagas || !confirmadas[p.TransacaoID] {
					integridade = false
				}
				vistos[p.Assento] = true
			}
		}
		if !encontrou {
			integridade = false
		}
	}
	m.calcularTempos(latencias, duracao)
	// Contencao pode abortar tentativas sem consumir todas as vagas; seguranca
	// nao deve ser confundida com exigir uma taxa artificial de confirmacoes.
	m.Integridade = integridade && m.Pendentes == 0 && m.ErrosTransporte == 0 && m.Confirmadas <= min(quantidade, vagas) && m.Confirmadas > 0
	if !m.Integridade {
		return m, fmt.Errorf("carga distribuída encontrou inconsistência, pendência ou falha de transporte")
	}
	return m, nil
}

// calcularTempos usa o mesmo cálculo de média, p95 e vazão nos dois cenários.
// Recebe: latencias em milissegundos, com uma entrada por cliente, e duração da fase medida.
// Retorna: nenhum valor; atualiza as métricas de tempo e ordena a lista recebida.
func (m *Metricas) calcularTempos(latencias []float64, duracao time.Duration) {
	m.LatenciaMediaMS = 0
	for _, latencia := range latencias {
		m.LatenciaMediaMS += latencia / float64(len(latencias))
	}
	sort.Float64s(latencias)
	m.LatenciaP95MS = latencias[int(math.Ceil(float64(len(latencias))*0.95))-1]
	m.DuracaoMS = float64(duracao) / float64(time.Millisecond)
	m.RequisicoesSegundo = float64(len(latencias)) / duracao.Seconds()
}
