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

type Metricas struct {
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

func chamar(endereco, acao, token string, dados any, alvo any) error {
	raw, err := json.Marshal(dados)
	if err != nil {
		return err
	}
	resposta, err := cliente.EnviarRequisicao(endereco, protocolo.Requisicao{Acao: acao, Token: token, Dados: raw})
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

func criarUsuario(endereco, email, perfil string) (string, error) {
	senha := "senha-carga-" + email
	if err := chamar(endereco, protocolo.AcaoCadastrar, "", protocolo.Cadastro{Nome: "Teste de carga", Email: email, Senha: senha, Perfil: perfil}, nil); err != nil {
		return "", err
	}
	var sessao protocolo.Sessao
	err := chamar(endereco, protocolo.AcaoEntrar, "", protocolo.Credenciais{Email: email, Senha: senha}, &sessao)
	return sessao.Token, err
}

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
		token, err := criarUsuario(endereco, fmt.Sprintf("m%d-%s@carga.test", i, prefixo), protocolo.Motorista)
		if err != nil {
			return m, err
		}
		motoristas[i] = token
		cidades := []string{"Carga-" + prefixo + "-A", "Carga-" + prefixo + "-B", "Carga-" + prefixo + "-C"}
		hora := "2099-10-01T08:00:00-03:00"
		if i == 1 {
			hora = "2099-10-01T09:15:00-03:00"
		}
		oferta := protocolo.PublicacaoCarona{ValorKM: 1, Chave: "carga", Rota: cidades[i : i+2], DataHora: hora, Assentos: vagas, Trechos: []protocolo.OfertaTrecho{{DistanciaKM: 10, TempoViagem: 60, TempoParada: 15}}}
		if err := chamar(endereco, protocolo.AcaoPublicar, token, oferta, &caronas[i]); err != nil {
			return m, err
		}
	}
	defer func() {
		for i, token := range motoristas {
			chamar(endereco, protocolo.AcaoCancelarCarona, token, protocolo.Identificador{ID: caronas[i].ID}, nil)
		}
	}()
	tokens := make([]string, quantidade)
	for i := range tokens {
		token, err := criarUsuario(endereco, fmt.Sprintf("p%d-%s@carga.test", i, prefixo), protocolo.Passageiro)
		if err != nil {
			return m, err
		}
		tokens[i] = token
	}
	defer func() {
		for _, token := range tokens {
			chamar(endereco, protocolo.AcaoSair, token, struct{}{}, nil)
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
	inicio := make(chan struct{})
	var wg sync.WaitGroup
	for i, token := range tokens {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-inicio
			instante := time.Now()
			resposta, err := cliente.EnviarRequisicao(endereco, protocolo.Requisicao{Acao: protocolo.AcaoConfirmar, Token: token, Dados: pedido})
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
		m.LatenciaMediaMS += latencias[i] / float64(quantidade)
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
	sort.Float64s(latencias)
	m.LatenciaP95MS = latencias[int(math.Ceil(float64(quantidade)*0.95))-1]
	m.DuracaoMS = float64(duracao) / float64(time.Millisecond)
	m.RequisicoesSegundo = float64(quantidade) / duracao.Seconds()
	for i, token := range motoristas {
		var publicadas []protocolo.Carona
		if err := chamar(endereco, protocolo.AcaoCaronas, token, struct{}{}, &publicadas); err != nil {
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
