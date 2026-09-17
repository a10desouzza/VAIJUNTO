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

// TestCadastroAutenticacaoAutorizacaoEDesconexao: recebe o teste e cria uma conta normalizada.
// Confere senha, duplicacao, perfis, sessao desconhecida e revogacao no logout. Sem retorno.
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
	if _, err := g.ConsultarCaronas("invalido"); err == nil {
		t.Fatal("sessão inválida aceita")
	}
	p := protocolo.PublicacaoCarona{Chave: "sem-permissao", Rota: []string{"A", "B"}, DataHora: "2099-10-01T09:00:00Z", Assentos: 1, Trechos: []protocolo.OfertaTrecho{{Preco: 10, DistanciaKM: 10, TempoViagem: 60}}}
	if _, err := g.Publicar(sessao.ID, p); err == nil {
		t.Fatal("passageiro publicou")
	}
	if err := g.Desconectar(sessao.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := g.ConsultarReservas(sessao.ID); err == nil {
		t.Fatal("sessão revogada aceita")
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
	}
	for i, busca := range casos {
		if _, err := c.g.Buscar(c.p, busca); err == nil {
			t.Errorf("caso %d foi aceito: %+v", i, busca)
		}
	}
	if _, err := c.g.ConsultarReservas(c.m1); err == nil {
		t.Fatal("motorista autorizado a consultar reservas de passageiro")
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
		c.reautenticar(t)
		if _, err := c.g.CancelarReserva(c.p, reserva.ID); err == nil {
			t.Fatal("reserva iniciada foi cancelada")
		}
		if c.g.reservas[reserva.ID].Status != protocolo.Ativa {
			t.Fatal("tentativa recusada alterou a reserva")
		}
	})
}

func TestPrecoPorTrechoValidacaoESoma(t *testing.T) {
	c := criarCenario(t)
	p := protocolo.PublicacaoCarona{Chave: "precos", Rota: []string{"Serrinha", "Feira", "Salvador"}, DataHora: c.agora.Add(time.Hour).Format(time.RFC3339), Assentos: 2, Trechos: []protocolo.OfertaTrecho{{Preco: 0.10, DistanciaKM: 999, TempoViagem: 60}, {Preco: 0.20, DistanciaKM: 1, TempoViagem: 60}}}
	for _, valor := range []float64{-1, 1.001, 1000000.01, math.NaN(), math.Inf(1)} {
		p.Trechos[0].Preco = valor
		if _, err := c.g.Publicar(c.m1, p); err == nil {
			t.Fatalf("preço inválido aceito: %v", valor)
		}
	}
	p.Trechos[0].Preco = 0.10
	carona, err := c.g.Publicar(c.m1, p)
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.g.Confirmar(c.p, protocolo.ReservaItinerario{Chave: "soma", TrechosIDs: []string{carona.Trechos[0].Trecho.ID, carona.Trechos[1].Trecho.ID}})
	if err != nil || r.PrecoTotal != 0.30 {
		t.Fatalf("soma em centavos: %+v %v", r, err)
	}
	p.Trechos[0].Preco = 0
	p.Chave = "gratuito"
	if _, err := c.g.Publicar(c.m1, p); err != nil {
		t.Fatal(err)
	}
}
