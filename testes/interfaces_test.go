// Autor: Arthur de Andrade Souza.
// Matéria: TEC502 - TP01 - Concorrência e Conectividade - 2026.2.
// Declaração de não plágio: Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
// Funcionalidade: Testa interfaces públicas, menus TCP, concorrência, autenticação REST e configuração de rede.
package testes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"vaijunto/internal/cliente"
	"vaijunto/internal/configuracao"
	"vaijunto/internal/consorcio"
	"vaijunto/internal/protocolo"
	"vaijunto/internal/servidor"
)

// TestEscolhaDaEmpresaAntesDoCadastro verifica a escolha da empresa antes de enviar credenciais ou cadastrar.
// Recebe: controle de teste t.
func TestEscolhaDaEmpresaAntesDoCadastro(t *testing.T) {
	nomes := []string{"VAIJUNTO", "Minas", "São Paulo"}
	for escolhida, nome := range nomes {
		t.Run(nome, func(t *testing.T) {
			g := servidor.NovoGrafo()
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancelar := context.WithCancel(context.Background())
			fim := make(chan error, 1)
			go func() {
				fim <- servidor.Servir(ctx, listener, g, time.Second)
			}()
			defer func() {
				cancelar()
				if err := <-fim; err != nil {
					t.Error(err)
				}
			}()
			empresas := make([]configuracao.ServidorCliente, len(nomes))
			for i, nome := range nomes {
				empresas[i] = configuracao.ServidorCliente{Nome: nome, Endereco: "127.0.0.1:1"}
			}
			empresas[escolhida].Endereco = listener.Addr().String()
			for _, perfil := range []string{protocolo.Motorista, protocolo.Passageiro} {
				email := fmt.Sprintf("%s@selecao.test", perfil)
				entrada := fmt.Sprintf("%d\n2\nPessoa Teste\n%s\nsenha1234\nsenha1234\n0\n", escolhida+1, email)
				var saida bytes.Buffer
				if err := cliente.ExecutarComSelecao(empresas, perfil, strings.NewReader(entrada), &saida); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(saida.String(), "Empresa: "+nome) || !strings.Contains(saida.String(), "Conta criada") {
					t.Fatal("escolha nao chegou ao cadastro", saida.String())
				}
				if _, err := g.Autenticar(protocolo.Credenciais{Email: email, Senha: "senha1234"}); err != nil {
					t.Fatal("conta foi enviada ao servidor errado", err)
				}
			}
		})
	}
}

// TestMenusFluxoCompletoTCP verifica o fluxo dos menus de motorista e passageiro pela interface TCP.
// Recebe: controle de teste t.
func TestMenusFluxoCompletoTCP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancelar := context.WithCancel(context.Background())
	fim := make(chan error, 1)
	go func() {
		fim <- servidor.Servir(ctx, listener, servidor.NovoGrafo(), time.Second)
	}()
	defer func() {
		cancelar()
		if err := <-fim; err != nil {
			t.Error(err)
		}
	}()
	executar := func(perfil, entrada string, esperados ...string) {
		t.Helper()
		var saida bytes.Buffer
		if err := cliente.ExecutarMenu(listener.Addr().String(), perfil, strings.NewReader(entrada), &saida); err != nil {
			t.Fatal(err)
		}
		for _, esperado := range esperados {
			if !strings.Contains(saida.String(), esperado) {
				t.Fatalf("não encontrou %q:\n%s", esperado, saida.String())
			}
		}
	}
	executar(protocolo.Motorista, "2\nAna\nana@menu.test\nsenha1234\nsenha1234\n1\n3\nSalvador\nFeira\nSerrinha\n2099-10-01\n08:00\n3\n100\n45,50\n120\n15\n80\n25\n60\n1\n2\n0\n", "Conta criada. Acesso realizado: Ana", "publicada com sucesso", "3/3 vagas")
	executar(protocolo.Passageiro, "2\nBia\nbia@menu.test\nsenha1234\nsenha1234\n1\nSalvador\nSerrinha\n2099-10-01\n1\n1\n2\n0\n", "Conta criada. Acesso realizado: Bia", "confirmada!", "R$ 70,50", "1 vaga reservada", "ATIVA")
	executar(protocolo.Motorista, "1\nana@menu.test\nsenha1234\n2\n0\n", "2/3 vagas", "Bia")
	executar(protocolo.Passageiro, "1\nbia@menu.test\nsenha1234\n3\n1\n1\n2\n0\n", "Os assentos foram devolvidos", "CANCELADA")
	executar(protocolo.Passageiro, "1\nbia@menu.test\nsenha1234\n1\nSalvador\nSerrinha\n2099-10-01\n1\n1\n0\n", "confirmada!")
	executar(protocolo.Motorista, "1\nana@menu.test\nsenha1234\n5\n1\n2\n1\n2\n0\n", "Trecho cancelado", "PARCIALMENTE_CANCELADA", "3/3 vagas")
	executar(protocolo.Passageiro, "1\nbia@menu.test\nsenha1234\n5\n2\n0\n", "Aviso da reserva", "itinerário inteiro cancelado", "CANCELADA")
	executar(protocolo.Motorista, "1\nana@menu.test\nsenha1234\n2\n3\n1\n1\n2\n0\n", "3/3 vagas", "Carona cancelada", "CANCELADA")
}

// TestConcorrenciaUltimoAssento verifica que pedidos concorrentes não confirmam a mesma última vaga.
// Recebe: controle de teste t.
func TestConcorrenciaUltimoAssento(t *testing.T) {
	a := preparar(t)
	c := publicar(t, a, a.motorista, "abc", []string{"A", "B", "C"}, "2099-10-01T08:00:00-03:00", 1)
	var sucessos atomic.Int32
	var wg sync.WaitGroup
	inicio := make(chan struct{})
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-inicio
			sessaoID := a.passageiro
			if i%2 == 1 {
				sessaoID = a.outroPassageiro
			}
			if _, err := a.g.Confirmar(sessaoID, protocolo.ReservaItinerario{Chave: fmt.Sprint(i), TrechosIDs: ids(c)}); err == nil {
				sucessos.Add(1)
			}
		}()
	}
	close(inicio)
	wg.Wait()
	if sucessos.Load() != 1 {
		t.Fatalf("reservas confirmadas: %d", sucessos.Load())
	}
	caronas, err := a.g.ConsultarCaronas(a.motorista)
	if err != nil {
		t.Fatal(err)
	}
	for _, trecho := range caronas[0].Trechos {
		if trecho.Trecho.Assentos != 0 || len(trecho.Passageiros) != 1 || trecho.Passageiros[0].Assento != 1 {
			t.Fatalf("overbooking: %+v", trecho)
		}
	}
}

// TestRespostaPerdidaPodeSerRecuperada verifica que repetir a chave recupera o resultado sem duplicar a reserva.
// Recebe: controle de teste t.
func TestRespostaPerdidaPodeSerRecuperada(t *testing.T) {
	a := preparar(t)
	c := publicar(t, a, a.motorista, "ab", []string{"A", "B"}, "2099-10-01T08:00:00-03:00", 1)
	pedido := protocolo.ReservaItinerario{Chave: "resposta-perdida", TrechosIDs: ids(c)}
	dados, _ := json.Marshal(pedido)
	mensagem, _ := json.Marshal(protocolo.Requisicao{Acao: protocolo.AcaoConfirmar, SessaoID: a.passageiro, Dados: dados})
	servidorConn, clienteConn := net.Pipe()
	fim := make(chan struct{})
	go func() {
		defer close(fim)
		servidor.AtenderConexao(servidorConn, a.g, time.Second)
	}()
	clienteConn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := clienteConn.Write(append(mensagem, '\n')); err != nil {
		t.Fatal(err)
	}
	clienteConn.Close()
	select {
	case <-fim:
	case <-time.After(2 * time.Second):
		t.Fatal("servidor preso no cliente desconectado")
	}
	r, err := a.g.Confirmar(a.passageiro, pedido)
	if err != nil {
		t.Fatal(err)
	}
	reservas, _ := a.g.ConsultarReservas(a.passageiro)
	if len(reservas) != 1 || reservas[0].ID != r.ID {
		t.Fatal("resposta perdida duplicou reserva")
	}
}

// TestAtomicidadeEAssentoPorTrecho verifica alocação integral e numeração de assentos por trecho.
// Recebe: controle de teste t.
func TestAtomicidadeEAssentoPorTrecho(t *testing.T) {
	a := preparar(t)
	c := publicar(t, a, a.motorista, "abc", []string{"A", "B", "C"}, "2099-10-01T08:00:00-03:00", 1)
	caminho := ids(c)
	for i, invalida := range [][]string{nil, {caminho[0], caminho[0]}, {caminho[0], "inexistente"}, {caminho[1], caminho[0]}} {
		if _, err := a.g.Confirmar(a.passageiro, protocolo.ReservaItinerario{Chave: fmt.Sprint(i), TrechosIDs: invalida}); err == nil {
			t.Fatalf("aceitou %v", invalida)
		}
	}
	if _, err := a.g.Confirmar(a.outroPassageiro, protocolo.ReservaItinerario{Chave: "bc", TrechosIDs: caminho[1:]}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.g.Confirmar(a.passageiro, protocolo.ReservaItinerario{Chave: "abc", TrechosIDs: caminho}); err == nil {
		t.Fatal("reserva parcial aceita")
	}
	if _, err := a.g.Confirmar(a.passageiro, protocolo.ReservaItinerario{Chave: "ab", TrechosIDs: caminho[:1]}); err != nil {
		t.Fatalf("primeiro trecho indevidamente bloqueado: %v", err)
	}
}

// TestRESTExigeTokenERejeitaIdentidadeDivergente verifica autenticação REST e rejeição de catálogo com identidade incorreta.
// Recebe: controle de teste t.
func TestRESTExigeTokenERejeitaIdentidadeDivergente(t *testing.T) {
	g := servidor.NovoGrafo()
	api := novaTeste(t, "empresa-a", g, nil)
	for _, path := range []string{"/saude", "/trechos", "/consorcio/trechos"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		api.Handler().ServeHTTP(w, r)
		if w.Code != http.StatusUnauthorized {
			t.Fatal("endpoint sem autenticacao")
		}
	}
	b := httptest.NewServer(novaTeste(t, "empresa-c", g, nil).Handler())
	defer b.Close()
	api = novaTeste(t, "empresa-a", g, []configuracao.Empresa{{ID: "empresa-b", URL: b.URL}})
	r := api.ConsultarTodos(context.Background())
	if len(r.Indisponiveis) != 1 {
		t.Fatal("identidade divergente aceita")
	}
}

// TestAgruparTrechosIdentificaEmpresaEPreservaOrdemLocal verifica agrupamento por dono sem confundir IDs locais nem reordenar seus trechos.
// Recebe: controle de teste t.
func TestAgruparTrechosIdentificaEmpresaEPreservaOrdemLocal(t *testing.T) {
	p := protocolo.ReservaDistribuida{Chave: "viagem", Trechos: []protocolo.ReferenciaTrecho{{EmpresaID: "empresa-a", TrechoID: "T1"}, {EmpresaID: "empresa-b", TrechoID: "T1"}, {EmpresaID: "empresa-a", TrechoID: "T2"}}}
	g, err := consorcio.AgruparTrechos(p)
	if err != nil {
		t.Fatal(err)
	}
	esperado := map[string][]string{"empresa-a": {"T1", "T2"}, "empresa-b": {"T1"}}
	if !reflect.DeepEqual(g, esperado) {
		t.Fatalf("agrupamento incorreto: %+v", g)
	}
	p.Trechos = append(p.Trechos, p.Trechos[0])
	if _, err := consorcio.AgruparTrechos(p); err == nil {
		t.Fatal("mesma referencia repetida aceita")
	}
}

// TestValidarEnderecoCliente verifica a validação de endereços TCP destinados aos clientes.
// Recebe: controle de teste t.
func TestValidarEnderecoCliente(t *testing.T) {
	for _, endereco := range []string{"172.16.103.7:8080", "localhost:7001", "empresa-a:8080", "[::1]:8080"} {
		if err := configuracao.ValidarEnderecoCliente(endereco); err != nil {
			t.Errorf("%s: %v", endereco, err)
		}
	}
	for _, endereco := range []string{"172.16.103.7:8081:8080", "http://172.16.103.7:8081", "", ":8080", "localhost:0", "localhost:65536", "localhost:abc"} {
		if err := configuracao.ValidarEnderecoCliente(endereco); err == nil {
			t.Errorf("endereco invalido aceito: %s", endereco)
		}
	}
}

// TestEmpresas verifica a interpretação dos pares e a rejeição de configuração inválida.
// Recebe: controle de teste t.
func TestEmpresas(t *testing.T) {
	for _, texto := range []string{"empresa-a=http://localhost:8001", "b=http://localhost:8002,b=http://localhost:8003", "b=ftp://localhost", "b=http://usuario:senha@localhost", "b=http://localhost/trechos", "b=sem-host"} {
		if _, err := configuracao.Empresas(texto, "empresa-a"); err == nil {
			t.Fatalf("configuracao invalida aceita: %s", texto)
		}
	}
	pares, err := configuracao.Empresas("empresa-b=http://localhost:8002,empresa-c=http://localhost:8003/", "empresa-a")
	if err != nil || len(pares) != 2 || pares[1].URL != "http://localhost:8003" {
		t.Fatalf("pares: %+v; erro: %v", pares, err)
	}
}

// novaTeste configura uma API de catálogo para os testes de interface pública.
// Recebe: controle t, identidade, fonte local e pares.
func novaTeste(t *testing.T, id string, g consorcio.Fonte, pares []configuracao.Empresa) *consorcio.API {
	t.Helper()
	a, err := consorcio.NovaAPI(id, tokenTeste, pares, g, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// oferta monta uma publicação de teste com preços e tempos conhecidos por trecho.
// Recebe: chave, rota, partida e vagas.
func oferta(chave string, rota []string, hora string, vagas int) protocolo.PublicacaoCarona {
	trechos := make([]protocolo.OfertaTrecho, len(rota)-1)
	for i := range trechos {
		trechos[i] = protocolo.OfertaTrecho{Preco: 10, DistanciaKM: 10, TempoViagem: 60, TempoParada: 15}
	}
	return protocolo.PublicacaoCarona{Chave: chave, Rota: rota, DataHora: hora, Assentos: vagas, Trechos: trechos}
}

// preparar cria o ambiente de teste com gerenciador e usuários autenticados.
// Recebe: controle de teste t.
func preparar(t testing.TB) ambiente {
	t.Helper()
	g := servidor.NovoGrafo()
	return ambiente{g: g, motorista: usuario(t, g, "m@teste.com", protocolo.Motorista), outroMotorista: usuario(t, g, "m2@teste.com", protocolo.Motorista), passageiro: usuario(t, g, "p@teste.com", protocolo.Passageiro), outroPassageiro: usuario(t, g, "p2@teste.com", protocolo.Passageiro)}
}

const tokenTeste = "token-local-testes-123456"
