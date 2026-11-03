// Autor: Arthur de Andrade Souza.
// Matéria: TEC502 - TP01 - Concorrência e Conectividade - 2026.2.
// Declaração de não plágio: Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
// Funcionalidade: Simula buscas e reservas concorrentes nas três empresas e verifica a integridade dos assentos.
package carga

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
	"vaijunto/internal/cliente"
	"vaijunto/internal/protocolo"
)

// ExecutarConsorcio gera buscas e reservas a partir das tres empresas. Confere os assentos locais contra as transacoes globais, e nao apenas respostas HTTP.
// Recebe: três endereços TCP, quantidade de clientes e vagas.
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

	m.Integridade = integridade && m.Pendentes == 0 && m.ErrosTransporte == 0 && m.Confirmadas <= min(quantidade, vagas) && m.Confirmadas > 0
	if !m.Integridade {
		return m, fmt.Errorf("carga distribuída encontrou inconsistência, pendência ou falha de transporte")
	}
	return m, nil
}
