// Arquivo: internal/servidor/distribuido_test.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Monta três empresas com comunicação HTTP local para testar o protocolo distribuído.
// Verifica disputa de vagas, repetição, cancelamento, carga TCP e recuperação de falhas.
// Inclui a disputa simultânea do mesmo itinerário por passageiros de origens diferentes.

package servidor

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"vaijunto/internal/carga"
	"vaijunto/internal/configuracao"
	"vaijunto/internal/consorcio"
	"vaijunto/internal/protocolo"
)

type empresaTeste struct {
	g                     *GrafoItinerarios
	d                     *Distribuido
	http                  *httptest.Server
	offline               atomic.Bool
	motorista, passageiro string
	carona                protocolo.Carona
}

// TestConsultaMantemReservaComEmpresaIndisponivel verifica o cenario: Consulta Mantem Reserva Com Empresa Indisponivel.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
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

// TestConsorcioCargaTCPDeTresEmpresas verifica o cenario: Consorcio Carga TCPDe Tres Empresas.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestConsorcioCargaTCPDeTresEmpresas(t *testing.T) {
	es := criarConsorcio(t, 1)
	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()
	enderecos := make([]string, 3)
	for i, e := range es {
		if err := e.g.AbrirBackup(e.d.id, filepath.Join(t.TempDir(), "estado.json")); err != nil {
			t.Fatal(err)
		}
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		enderecos[i] = listener.Addr().String()
		fim := make(chan error, 1)
		fimRecuperacao := make(chan struct{})
		go func() {
			fim <- Servir(ctx, listener, e.g, 5*time.Second)
		}()
		t.Cleanup(func() {
			cancelar()
			if err := <-fim; err != nil {
				t.Error(err)
			}
			<-fimRecuperacao
			e.http.Close()
		})
		go func() {
			defer close(fimRecuperacao)
			e.d.Recuperar(ctx)
		}()
	}
	m, err := carga.ExecutarConsorcio(enderecos, 30, 2)
	if err != nil {
		t.Fatalf("carga: %+v %v", m, err)
	}
	t.Logf("carga em três empresas: %+v", m)
}

// TestConsorcioReservaLocalIndependeDosOutrosServidores verifica o cenario: Consorcio Reserva Local Independe Dos Outros Servidores.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
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

// TestConsorcioCancelamentoPeloMotoristaLiberaTodosOsTrechos verifica o cenario: Consorcio Cancelamento Pelo Motorista Libera Todos Os Trechos.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestConsorcioCancelamentoPeloMotoristaLiberaTodosOsTrechos(t *testing.T) {
	es := criarConsorcio(t, 1)
	r, err := es[0].d.reservar(es[0].passageiro, pedidoTeste(es, "motorista"))
	if err != nil || r.Status != protocolo.Ativa {
		t.Fatal(err)
	}
	if _, err := es[1].d.cancelarOferta(es[1].motorista, es[1].carona.ID, false); err != nil {
		t.Fatal(err)
	}
	verificarVagas(t, es, 1, 0, 0)
	if es[0].d.reservaOrigem(r.ID).Status != protocolo.Cancelada {
		t.Fatal("origem nao recebeu o cancelamento")
	}
}

// criarConsorcio monta tres empresas de teste com contas, ofertas conectadas e servidores HTTP locais.
// Recebe: t *testing.T, vagas int.
// Retorna: []*empresaTeste.
func criarConsorcio(t *testing.T, vagas int) []*empresaTeste {
	t.Helper()
	es := make([]*empresaTeste, 3)
	muxes := make([]*http.ServeMux, 3)
	for i := range es {
		e := &empresaTeste{g: NovoGrafo()}
		es[i] = e
		mux := http.NewServeMux()
		muxes[i] = mux
		e.http = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if e.offline.Load() {
				http.Error(w, "falha simulada", 503)
				return
			}
			mux.ServeHTTP(w, r)
		}))
		t.Cleanup(e.http.Close)
	}
	for i, e := range es {
		id := fmt.Sprintf("empresa-%d", i)
		pares := make([]configuracao.Empresa, 0, 2)
		for j, outro := range es {
			if i != j {
				pares = append(pares, configuracao.Empresa{ID: fmt.Sprintf("empresa-%d", j), URL: "http://" + outro.http.Listener.Addr().String()})
			}
		}
		api, err := consorcio.NovaAPI(id, "token-consorcio-teste-123", pares, e.g, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		e.d, err = NovoDistribuido(e.g, id, "token-consorcio-teste-123", pares, api, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		muxes[i].Handle("/", api.Handler())
		muxes[i].Handle("/distribuido/", e.d.Handler())
		e.http.Start()
		for _, perfil := range []string{protocolo.Motorista, protocolo.Passageiro} {
			email := fmt.Sprintf("%s@empresa%d.test", perfil, i)
			_, err := e.g.Cadastrar(protocolo.Cadastro{Nome: "Pessoa Teste", Email: email, Senha: "senha1234", Perfil: perfil})
			if err != nil {
				t.Fatal(err)
			}
			s, err := e.g.Autenticar(protocolo.Credenciais{Email: email, Senha: "senha1234"})
			if err != nil {
				t.Fatal(err)
			}
			if perfil == protocolo.Motorista {
				e.motorista = s.ID
			} else {
				e.passageiro = s.ID
			}
		}
		e.carona, err = e.g.Publicar(e.motorista, protocolo.PublicacaoCarona{Chave: "oferta", Rota: []string{fmt.Sprintf("Cidade %d", i), fmt.Sprintf("Cidade %d", i+1)}, DataHora: fmt.Sprintf("2099-10-01T%02d:00:00-03:00", 8+i), Assentos: vagas, Trechos: []protocolo.OfertaTrecho{{Preco: 20, DistanciaKM: 10, TempoViagem: 30}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	return es
}

// pedidoTeste monta um pedido de reserva com referencias aos trechos das empresas do cenario.
// Recebe: es []*empresaTeste, chave string.
// Retorna: protocolo.ReservaItinerario.
func pedidoTeste(es []*empresaTeste, chave string) protocolo.ReservaItinerario {
	p := protocolo.ReservaItinerario{Chave: chave}
	for i, e := range es {
		p.Trechos = append(p.Trechos, protocolo.ReferenciaTrecho{EmpresaID: fmt.Sprintf("empresa-%d", i), TrechoID: e.carona.Trechos[0].Trecho.ID})
	}
	return p
}

// verificarVagas compara vagas livres, ocupadas e preparadas com os valores esperados em todas as empresas.
// Recebe: t *testing.T, es []*empresaTeste, livres, ocupados, preparados int.
// Retorna: nenhum valor; executa os efeitos descritos acima.
func verificarVagas(t *testing.T, es []*empresaTeste, livres, ocupados, preparados int) {
	t.Helper()
	for _, e := range es {
		e.g.mu.RLock()
		s := e.g.trechos[e.carona.Trechos[0].Trecho.ID]
		ok := s.trecho.Assentos == livres && len(s.ocupados) == ocupados && len(s.preparados) == preparados
		e.g.mu.RUnlock()
		if !ok {
			t.Fatalf("%s: livres=%d ocupados=%d preparados=%d", e.d.id, s.trecho.Assentos, len(s.ocupados), len(s.preparados))
		}
	}
}

// TestConsorcioBuscaReservaRepeticaoECancelamento verifica o cenario: Consorcio Busca Reserva Repeticao ECancelamento.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestConsorcioBuscaReservaRepeticaoECancelamento(t *testing.T) {
	es := criarConsorcio(t, 2)
	d := es[0].d
	b, err := d.buscar(es[0].passageiro, protocolo.BuscaItinerario{Origem: "Cidade 0", Destino: "Cidade 3", Data: "2099-10-01"})
	if err != nil || len(b.Itinerarios) != 1 || len(b.Itinerarios[0].Trechos) != 3 {
		t.Fatalf("busca: %+v %v", b, err)
	}
	p := pedidoTeste(es, "viagem")
	r, err := d.reservar(es[0].passageiro, p)
	if err != nil || r.Status != protocolo.Ativa {
		t.Fatalf("reserva: %+v %v", r, err)
	}
	verificarVagas(t, es, 1, 1, 0)
	repetida, err := d.reservar(es[0].passageiro, p)
	if err != nil || repetida.ID != r.ID {
		t.Fatal("repeticao duplicou reserva")
	}
	verificarVagas(t, es, 1, 1, 0)
	for _, e := range es {
		cs, err := e.g.ConsultarCaronas(e.motorista)
		if err != nil || len(cs[0].Trechos[0].Passageiros) != 1 {
			t.Fatal("motorista nao consulta passageiro externo")
		}
	}
	if _, err := d.cancelar(es[0].passageiro, r.ID); err != nil {
		t.Fatal(err)
	}
	verificarVagas(t, es, 2, 0, 0)
	if d.reservaOrigem(r.ID).Status != protocolo.Cancelada {
		t.Fatal("reserva global nao foi cancelada")
	}
}

// TestConsorcioUltimaVagaDisputadaPorTresOrigens verifica o cenario: Consorcio Ultima Vaga Disputada Por Tres Origens.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestConsorcioUltimaVagaDisputadaPorTresOrigens(t *testing.T) {
	es := criarConsorcio(t, 1)
	var wg sync.WaitGroup
	var confirmadas atomic.Int32
	inicio := make(chan struct{})
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-inicio
			e := es[i%3]
			r, err := e.d.reservar(e.passageiro, pedidoTeste(es, fmt.Sprint(i)))
			if err == nil && r.Status == protocolo.Ativa {
				confirmadas.Add(1)
			}
		}(i)
	}
	close(inicio)
	wg.Wait()
	if confirmadas.Load() != 1 {
		t.Fatalf("confirmacoes: %d", confirmadas.Load())
	}
	verificarVagas(t, es, 0, 1, 0)
}

// As duas origens disputam o mesmo itinerario A+B, com um dono por trecho.
// Sem falha de rede, uma compra precisa terminar e a outra deve ser recusada.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestConsorcioDisputaDoisTrechosEntreDuasOrigens(t *testing.T) {
	empresas := criarConsorcio(t, 1)
	participantes := empresas[:2]
	inicio := make(chan struct{})
	type resultado struct {
		reserva protocolo.Reserva
		err     error
	}
	resultados := make(chan resultado, 2)
	for _, origem := range participantes {
		go func() {
			<-inicio
			r, err := origem.d.reservar(origem.passageiro, pedidoTeste(participantes, "duelo"))
			resultados <- resultado{r, err}
		}()
	}
	close(inicio)
	confirmadas := 0
	var vencedora protocolo.Reserva
	for range participantes {
		r := <-resultados
		if r.err == nil && r.reserva.Status == protocolo.Ativa {
			confirmadas++
			vencedora = r.reserva
		} else if r.err == nil && r.reserva.Status == pendente {
			t.Fatal("disputa sem falha terminou pendente")
		}
	}
	if confirmadas != 1 {
		t.Fatalf("queria uma compra completa; confirmacoes=%d", confirmadas)
	}
	verificarVagas(t, participantes, 0, 1, 0)
	verificarVagas(t, empresas[2:], 1, 0, 0)
	for _, e := range participantes {
		e.g.mu.RLock()
		assento := e.g.trechos[e.carona.Trechos[0].Trecho.ID].ocupados[1]
		transacao := e.g.reservas[assento].TransacaoID
		e.g.mu.RUnlock()
		if transacao != vencedora.TransacaoID {
			t.Fatal("cada dono confirmou um passageiro diferente")
		}
	}
	t.Logf("itinerario confirmado para a origem %s, transacao=%s", vencedora.EmpresaOrigem, vencedora.TransacaoID)
}

// TestConsorcioRecuperaQuedaAntesEDepoisDaEscolha verifica o cenario: Consorcio Recupera Queda Antes EDepois Da Escolha.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
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
			// O processo A ainda conserva RAM, mas nao responde. A recuperacao usa B+C.
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

// TestConsorcioNaoLiberaPreparacaoSemMaioria verifica o cenario: Consorcio Nao Libera Preparacao Sem Maioria.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
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

// TestConsorcioProcessoNovoRecusaVotosDaGeracaoPerdida verifica o cenario: Consorcio Processo Novo Recusa Votos Da Geracao Perdida.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
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
	// Outra identificacao simula nova RAM; nao pode votar usando o historico antigo.
	antiga := a.d.geracao
	a.d.geracao = "novo-processo"
	defer func() {
		a.d.geracao = antiga
	}()
	if _, err := a.d.consenso("prometer", mensagemConsenso{Meta: meta, Cedula: cedula{100, a.d.id}}); err == nil {
		t.Fatal("processo reiniciado votou em transacao da geracao anterior")
	}
}
