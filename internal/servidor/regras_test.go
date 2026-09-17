// internal/servidor/regras_test.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Testes de horario, preco, cancelamento e notificacoes.
// O relogio do cenario permite simular o inicio do percurso sem esperar o tempo real.

package servidor

import (
	"testing"
	"time"
	"vaijunto/internal/protocolo"
)

type cenarioViagem struct {
	g     *GrafoItinerarios
	agora time.Time
	m1    string
	m2    string
	p     string
	outro string
	ab    protocolo.Carona
	bc    protocolo.Carona
}

// criarCenario: Cria um grafo novo e injeta um relogio ajustavel. Publica A-B com um motorista e
// B-C-D com outro. Retorna: Ponteiro para cenarioViagem com relogio, contas e caronas preparadas.
func criarCenario(t *testing.T) *cenarioViagem {
	t.Helper()
	c := &cenarioViagem{g: NovoGrafo(), agora: time.Date(2099, 10, 1, 8, 0, 0, 0, time.FixedZone("Bahia", -3*3600))}
	c.g.agora = func() time.Time { return c.agora }
	usuario := func(email, perfil string) string {
		t.Helper()
		if _, err := c.g.Cadastrar(protocolo.Cadastro{Nome: "Pessoa Teste", Email: email, Senha: "senha1234", Perfil: perfil}); err != nil {
			t.Fatal(err)
		}
		s, err := c.g.Autenticar(protocolo.Credenciais{Email: email, Senha: "senha1234"})
		if err != nil {
			t.Fatal(err)
		}
		return s.ID
	}
	c.m1 = usuario("m1@teste.com", protocolo.Motorista)
	c.m2 = usuario("m2@teste.com", protocolo.Motorista)
	c.p = usuario("p@teste.com", protocolo.Passageiro)
	c.outro = usuario("outro@teste.com", protocolo.Passageiro)
	publicar := func(sessaoID, chave string, rota []string, minutos int) protocolo.Carona {
		t.Helper()
		ofertas := make([]protocolo.OfertaTrecho, len(rota)-1)
		for i := range ofertas {
			ofertas[i] = protocolo.OfertaTrecho{Preco: 20, DistanciaKM: 10, TempoViagem: 10, TempoParada: 5}
		}
		carona, err := c.g.Publicar(sessaoID, protocolo.PublicacaoCarona{Chave: chave, Rota: rota, DataHora: c.agora.Add(time.Duration(minutos) * time.Minute).Format(time.RFC3339), Assentos: 2, Trechos: ofertas})
		if err != nil {
			t.Fatal(err)
		}
		return carona
	}
	c.ab = publicar(c.m1, "ab", []string{"A", "B"}, 30)
	c.bc = publicar(c.m2, "bcd", []string{"B", "C", "D"}, 50)
	return c
}

// reservar: Confirma o percurso formado pelo primeiro trecho de cada uma das duas caronas.
// Retorna: Reserva de A-B-C. Interrompe o teste com Fatal se a confirmacao falhar.
func (c *cenarioViagem) reservar(t *testing.T) protocolo.Reserva {
	t.Helper()
	r, err := c.g.Confirmar(c.p, protocolo.ReservaItinerario{Chave: "abc", TrechosIDs: []string{c.ab.Trechos[0].Trecho.ID, c.bc.Trechos[0].Trecho.ID}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// Viagens podem durar mais que a sessao: autentica novamente apos avancar o relogio.
func (c *cenarioViagem) reautenticar(t *testing.T) {
	t.Helper()
	for email, destino := range map[string]*string{"m1@teste.com": &c.m1, "m2@teste.com": &c.m2, "p@teste.com": &c.p, "outro@teste.com": &c.outro} {
		s, err := c.g.Autenticar(protocolo.Credenciais{Email: email, Senha: "senha1234"})
		if err != nil {
			t.Fatal(err)
		}
		*destino = s.ID
	}
}

// cancelarConexao: recebe o tipo de cancelamento e solicita ao segundo motorista que cancele
// a carona ou seu primeiro trecho. Retorna: erro do servidor ou nil se o cancelamento ocorreu.
func (c *cenarioViagem) cancelarConexao(tipo string) error {
	if tipo == "trecho" {
		_, err := c.g.CancelarTrecho(c.m2, c.bc.Trechos[0].Trecho.ID)
		return err
	}
	_, err := c.g.CancelarCarona(c.m2, c.bc.ID)
	return err
}
