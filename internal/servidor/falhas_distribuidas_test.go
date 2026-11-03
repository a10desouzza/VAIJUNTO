// TEC502 - Problema 2. Autor: Arthur de Andrade Souza.
// Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
package servidor

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"vaijunto/internal/protocolo"
)

func TestCancelamentoMotoristaDoisRespeitaInicioDoItinerario(t *testing.T) {
	for _, tipo := range []string{"trecho", "carona"} {
		for _, deslocamento := range []time.Duration{-time.Second, 0, time.Second} {
			t.Run(fmt.Sprintf("%s/%s", tipo, deslocamento), func(t *testing.T) {
				c := criarCenario(t)
				r := c.reservar(t)
				c.agora = partida(r.Assentos[0].Trecho).Add(deslocamento)
				c.reautenticar(t)
				if tipo == "carona" {
					if _, err := c.g.CancelarCarona(c.m1, c.bc.ID); err == nil {
						t.Fatal("outro motorista cancelou a carona")
					}
				}
				err := c.cancelarConexao(tipo)
				if deslocamento >= 0 {
					if err == nil || !strings.Contains(err.Error(), "já iniciou o itinerário") {
						t.Fatalf("deveria proteger conexão: %v", err)
					}
					if c.g.reservas[r.ID].Status != protocolo.Ativa || c.g.caronas[c.bc.ID].status != protocolo.Ativa || len(c.g.notificacoes[r.Passageiro]) != 0 {
						t.Fatal("cancelamento recusado alterou reserva ou avisos")
					}
					for _, a := range r.Assentos {
						trecho := c.g.trechos[a.Trecho.ID].trecho
						if trecho.Status != protocolo.Ativa || trecho.Assentos != 1 {
							t.Fatal("cancelamento recusado alterou trecho ou vagas")
						}
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if c.g.reservas[r.ID].Status != protocolo.Cancelada {
					t.Fatal("nao cancelou A-B-C por inteiro")
				}
				if err := c.cancelarConexao(tipo); err != nil {
					t.Fatal(err)
				}
				for _, a := range r.Assentos {
					if c.g.trechos[a.Trecho.ID].trecho.Assentos != 2 || len(c.g.trechos[a.Trecho.ID].ocupados) != 0 {
						t.Fatal("vagas nao devolvidas exatamente uma vez")
					}
				}
				if c.g.caronas[c.ab.ID].status != protocolo.Ativa || c.g.trechos[c.ab.Trechos[0].Trecho.ID].trecho.Status != protocolo.Ativa {
					t.Fatal("oferta do motorista 1 foi cancelada indevidamente")
				}
				busca, err := c.g.Buscar(c.p, protocolo.BuscaItinerario{Origem: "B", Destino: "C", Data: "2099-10-01"})
				if err != nil || len(busca.Itinerarios) != 0 {
					t.Fatal("trecho cancelado apareceu na busca", err)
				}
				avisos, err := c.g.ConsultarNotificacoes(c.p)
				if err != nil || len(avisos) != 1 || avisos[0].ReservaID != r.ID {
					t.Fatalf("aviso de cancelamento: %+v %v", avisos, err)
				}
			})
		}
	}
}

func TestConsultaMantemReservaComEmpresaIndisponivel(t *testing.T) {
	empresas := criarConsorcio(t, 1)
	a := empresas[0]
	confirmada, err := a.d.reservar(a.passageiro, pedidoTeste(empresas, "indisponivel"))
	if err != nil || confirmada.Status != protocolo.Ativa {
		t.Fatal("reserva inicial", err)
	}
	empresas[1].offline.Store(true)
	reservas, err := a.d.consultar(a.passageiro)
	if err != nil || len(reservas) != 1 || reservas[0].Status != protocolo.Ativa || len(reservas[0].EmpresasIndisponiveis) != 1 || reservas[0].EmpresasIndisponiveis[0] != empresas[1].d.id {
		t.Fatalf("indisponibilidade alterou a reserva: %+v %v", reservas, err)
	}
}

func TestConsorcioReservaLocalIndependeDosOutrosServidores(t *testing.T) {
	es := criarConsorcio(t, 1)
	a := es[0]
	es[1].offline.Store(true)
	es[2].offline.Store(true)
	p := protocolo.ReservaItinerario{Chave: "local", Trechos: []protocolo.ReferenciaTrecho{{EmpresaID: a.d.id, TrechoID: a.carona.Trechos[0].Trecho.ID}}}
	r, err := a.d.reservar(a.passageiro, p)
	if err != nil || r.Status != protocolo.Ativa {
		t.Fatalf("reserva local dependeu dos pares: %+v %v", r, err)
	}
	if len(a.d.registros) != 0 {
		t.Fatal("reserva local criou consenso desnecessário")
	}
}

func TestConsorcioRecuperaQuedaAntesEDepoisDaEscolha(t *testing.T) {
	for _, escolhida := range []bool{false, true} {
		t.Run(fmt.Sprint(escolhida), func(t *testing.T) {
			es := criarConsorcio(t, 1)
			a, b := es[0], es[1]
			falhar := func() error {
				a.offline.Store(true)
				return fmt.Errorf("coordenador interrompido")
			}
			if escolhida {
				a.d.aposQuorum = falhar
			} else {
				a.d.aposPreparar = falhar
			}
			_, _ = a.d.reservar(a.passageiro, pedidoTeste(es, "falha"))
			verificarVagas(t, es, 0, 0, 1)
			b.d.abortarApos = 0
			b.d.recuperar(context.Background())
			for _, e := range es[1:] {
				e.g.mu.RLock()
				s := e.g.trechos[e.carona.Trechos[0].Trecho.ID]
				n := len(s.preparados)
				e.g.mu.RUnlock()
				if n != 0 {
					t.Fatal("vagas continuaram preparadas no servidor sobrevivente")
				}
			}
			a.offline.Store(false)
			b.d.recuperar(context.Background())
			a.d.aposQuorum = nil
			a.d.aposPreparar = nil
			a.d.recuperar(context.Background())
			if escolhida {
				verificarVagas(t, es, 0, 1, 0)
			} else {
				verificarVagas(t, es, 1, 0, 0)
			}
		})
	}
}

func TestConsorcioNaoLiberaPreparacaoSemMaioria(t *testing.T) {
	es := criarConsorcio(t, 1)
	a := es[0]
	a.d.aposPreparar = func() error {
		return fmt.Errorf("interrompido")
	}
	_, _ = a.d.reservar(a.passageiro, pedidoTeste(es, "sem-maioria"))
	es[1].offline.Store(true)
	es[2].offline.Store(true)
	a.d.abortarApos = 0
	a.d.recuperar(context.Background())
	verificarVagas(t, es, 0, 0, 1)
	es[1].offline.Store(false)
	es[2].offline.Store(false)
	a.d.recuperar(context.Background())
	verificarVagas(t, es, 1, 0, 0)
}

func TestConsorcioProcessoNovoRecusaVotosDaGeracaoPerdida(t *testing.T) {
	es := criarConsorcio(t, 1)
	a := es[0]
	a.d.aposPreparar = func() error {
		return fmt.Errorf("interrompido")
	}
	_, _ = a.d.reservar(a.passageiro, pedidoTeste(es, "reinicio"))
	a.d.mu.Lock()
	var meta metaTransacao
	for _, r := range a.d.registros {
		meta = copiaJSON(r.Meta)
		break
	}
	a.d.mu.Unlock()
	antiga := a.d.geracao
	a.d.geracao = "novo-processo"
	defer func() {
		a.d.geracao = antiga
	}()
	if _, err := a.d.consenso("prometer", mensagemConsenso{Meta: meta, Cedula: cedula{100, a.d.id}}); err == nil {
		t.Fatal("processo reiniciado votou em transacao da geracao anterior")
	}
}

func TestCargaComQuedaERecuperacao(t *testing.T) {
	empresas := criarConsorcio(t, 2)
	donos := empresas[:2]
	origem := empresas[2]
	caminho := filepath.Join(t.TempDir(), "origem.json")
	if err := origem.g.AbrirBackup(origem.d.id, caminho); err != nil {
		t.Fatal(err)
	}
	comprar := func(e *empresaTeste, chave string) (protocolo.Reserva, error) {
		return e.d.reservar(e.passageiro, pedidoTeste(donos, chave))
	}
	anterior, err := comprar(donos[0], "antes")
	if err != nil || anterior.Status != protocolo.Ativa {
		t.Fatalf("reserva inicial: %v %v", anterior.Status, err)
	}

	const concorrentes = 24
	inicio := make(chan struct{})
	resultados := make(chan protocolo.Reserva, concorrentes)
	for i := 0; i < concorrentes; i++ {
		go func() {
			<-inicio
			r, _ := comprar(donos[i%2], fmt.Sprintf("disputa-%d", i))
			resultados <- r
		}()
	}
	falha := fmt.Errorf("queda após preparar")
	origem.d.aposPreparar = func() error {
		verificarVagas(t, donos, 0, 1, 1)
		close(inicio)
		origem.http.Close()
		return falha
	}
	interrompida, err := comprar(origem, "interrompida")
	if err != falha {
		t.Fatalf("a falha programada não ocorreu: %v", err)
	}
	for i := 0; i < concorrentes; i++ {
		if r := <-resultados; r.Status == protocolo.Ativa {
			t.Fatalf("compra confirmada sem vaga: %s", r.ID)
		}
	}

	reiniciarEmpresaComBackup(t, origem, caminho)
	origem.d.abortarApos = 0
	ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	origem.d.recuperar(ctx)
	repetida, err := comprar(origem, "interrompida")
	if err != nil || repetida.TransacaoID != interrompida.TransacaoID || repetida.Status != protocolo.Cancelada {
		t.Fatalf("decisão após recuperação: %+v, erro %v", repetida, err)
	}
	verificarVagas(t, donos, 1, 1, 0)
	nova, err := comprar(donos[1], "apos-reinicio")
	if err != nil || nova.Status != protocolo.Ativa {
		t.Fatalf("vaga liberada não pôde ser comprada: %v %v", nova.Status, err)
	}
	verificarVagas(t, donos, 0, 2, 0)
	for _, e := range donos {
		trecho := e.g.trechos[e.carona.Trechos[0].Trecho.ID]
		contagem := make(map[string]int)
		for _, id := range trecho.ocupados {
			contagem[e.g.reservas[id].TransacaoID]++
		}
		if anterior.TransacaoID == nova.TransacaoID || contagem[anterior.TransacaoID] != 1 || contagem[nova.TransacaoID] != 1 {
			t.Fatalf("%s: ocupações por transação após recuperação: %v", e.d.id, contagem)
		}
	}
}
