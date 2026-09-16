package servidor

import (
	"math"
	"testing"
	"time"
	"vaijunto/internal/protocolo"
)

func TestAuxiliaresDeValoresCidadesEConexao(t *testing.T) {
	for _, caso := range []struct {
		valor  float64
		limite float64
		valido bool
	}{
		{0, 10, true}, {10, 10, true}, {1.25, 10, true},
		{-0.01, 10, false}, {10.01, 10, false}, {1.001, 10, false},
		{math.NaN(), 10, false}, {math.Inf(1), 10, false},
	} {
		if obtido := decimalValido(caso.valor, caso.limite); obtido != caso.valido {
			t.Errorf("decimalValido(%v, %v) = %v", caso.valor, caso.limite, obtido)
		}
	}
	if cidade("  Feira   de Santana ") != "Feira de Santana" || chaveCidade(" SÃO  PAULO ") != "são paulo" {
		t.Fatal("normalização de cidade incorreta")
	}
	if centavos(12.345) != 1235 {
		t.Fatalf("centavos(12.345) = %d", centavos(12.345))
	}

	a := protocolo.Trecho{Origem: "A", Destino: " Feira de Santana ", DataHora: "2099-10-01T08:00:00-03:00", TempoViagem: 30, TempoParada: 10}
	b := protocolo.Trecho{Origem: "feira DE santana", Destino: "B", DataHora: "2099-10-01T08:40:00-03:00"}
	if !conectam(a, b) {
		t.Fatal("conexão no horário exato foi recusada")
	}
	b.DataHora = "2099-10-01T08:39:59-03:00"
	if conectam(a, b) {
		t.Fatal("conexão antes do fim da parada foi aceita")
	}
}

func TestCadastroAutenticacaoAutorizacaoEDesconexao(t *testing.T) {
	g := NovoGrafo()
	g.agora = func() time.Time { return time.Date(2099, 10, 1, 8, 0, 0, 0, time.UTC) }
	cadastro := protocolo.Cadastro{Nome: "  Ana Teste  ", Email: " ANA@EXEMPLO.COM ", Senha: "senha1234", Perfil: protocolo.Passageiro}
	u, err := g.Cadastrar(cadastro)
	if err != nil {
		t.Fatal(err)
	}
	if u.Nome != "Ana Teste" || u.Email != "ana@exemplo.com" {
		t.Fatalf("cadastro não normalizado: %+v", u)
	}
	if _, err := g.Cadastrar(cadastro); err == nil {
		t.Fatal("cadastro duplicado foi aceito")
	}
	for _, credenciais := range []protocolo.Credenciais{
		{Email: "inexistente@exemplo.com", Senha: "senha1234"},
		{Email: u.Email, Senha: "incorreta"},
		{Email: u.Email, Senha: string(make([]byte, 129))},
	} {
		if _, err := g.Autenticar(credenciais); err == nil {
			t.Fatalf("credenciais inválidas aceitas: %+v", credenciais)
		}
	}
	sessao, err := g.Autenticar(protocolo.Credenciais{Email: " ANA@EXEMPLO.COM ", Senha: "senha1234"})
	if err != nil {
		t.Fatal(err)
	}
	if sessao.ID == "" || sessao.Usuario.Email != u.Email {
		t.Fatalf("sessão inválida: %+v", sessao)
	}
	if _, err := g.ConsultarCaronas(sessao.ID); err == nil {
		t.Fatal("passageiro autorizado como motorista")
	}
	if err := g.Desconectar(sessao.ID); err != nil {
		t.Fatal(err)
	}
	if err := g.Desconectar(sessao.ID); err == nil {
		t.Fatal("sessão desconectada continuou válida")
	}
}

func TestBuscarValidaParametrosAntesDeExplorarOGrafo(t *testing.T) {
	c := criarCenario(t)
	casos := []protocolo.BuscaItinerario{
		{Origem: "", Destino: "B", Data: "2099-10-01"},
		{Origem: "A", Destino: " a ", Data: "2099-10-01"},
		{Origem: "A", Destino: "B", Data: "01/10/2099"},
		{Origem: "A", Destino: "B", Data: "2099-10-01", MaxTrechos: -1},
		{Origem: "A", Destino: "B", Data: "2099-10-01", MaxTrechos: 21},
		{Origem: "A", Destino: "B", Data: "2099-10-01", OrdenarPor: "DISTANCIA"},
	}
	for i, busca := range casos {
		if _, err := c.g.Buscar(c.p, busca); err == nil {
			t.Errorf("caso %d foi aceito: %+v", i, busca)
		}
	}
	if _, err := c.g.Buscar(c.m1, protocolo.BuscaItinerario{Origem: "A", Destino: "B", Data: "2099-10-01"}); err == nil {
		t.Fatal("motorista autorizado a buscar como passageiro")
	}
}

func TestCancelarReservaValidaDonoHorarioEIdempotencia(t *testing.T) {
	t.Run("dono e repetição", func(t *testing.T) {
		c := criarCenario(t)
		reserva := c.reservar(t)
		if _, err := c.g.CancelarReserva(c.outro, reserva.ID); err == nil {
			t.Fatal("outro passageiro cancelou a reserva")
		}
		cancelada, err := c.g.CancelarReserva(c.p, reserva.ID)
		if err != nil {
			t.Fatal(err)
		}
		if cancelada.Status != protocolo.Cancelada || cancelada.Motivo == "" || cancelada.CanceladaEm == "" {
			t.Fatalf("reserva cancelada incompleta: %+v", cancelada)
		}
		for i := 0; i < 2; i++ {
			repetida, err := c.g.CancelarReserva(c.p, reserva.ID)
			if err != nil || repetida.ID != reserva.ID {
				t.Fatalf("repetição %d: reserva=%+v erro=%v", i, repetida, err)
			}
		}
		for _, assento := range reserva.Assentos {
			if obtido := c.g.trechos[assento.Trecho.ID].Assentos; obtido != 2 {
				t.Fatalf("assentos após repetição = %d; esperado 2", obtido)
			}
		}
	})

	t.Run("viagem iniciada", func(t *testing.T) {
		c := criarCenario(t)
		reserva := c.reservar(t)
		c.agora = partida(reserva.Assentos[0].Trecho)
		if _, err := c.g.CancelarReserva(c.p, reserva.ID); err == nil {
			t.Fatal("reserva iniciada foi cancelada")
		}
		if c.g.reservas[reserva.ID].Status != protocolo.Ativa {
			t.Fatal("tentativa recusada alterou a reserva")
		}
	})
}
