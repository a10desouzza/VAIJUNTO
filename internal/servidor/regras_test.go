// Arquivo: internal/servidor/regras_test.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Monta o cenário local e verifica cadastro, sessões, autorização e regras de viagem.
// Confere preços, limites de busca, cancelamento após a partida e logs sem segredos.

package servidor

import (
	"bytes"
	"log"
	"math"
	"strings"
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
// Recebe: t *testing.T.
// Retorna: *cenarioViagem.
func criarCenario(t *testing.T) *cenarioViagem {
	t.Helper()
	c := &cenarioViagem{g: NovoGrafo(), agora: time.Date(2099, 10, 1, 8, 0, 0, 0, time.FixedZone("Bahia", -3*3600))}
	c.g.agora = func() time.Time {
		return c.agora
	}
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
// Retorna: a reserva agregada, que pode estar ATIVA, PENDENTE ou cancelada, ou erro de validacao/execucao.
// Recebe: t *testing.T.
// Retorna: a reserva agregada, que pode estar ATIVA, PENDENTE ou cancelada, ou erro de validacao/execucao.
func (c *cenarioViagem) reservar(t *testing.T) protocolo.Reserva {
	t.Helper()
	r, err := c.g.Confirmar(c.p, protocolo.ReservaItinerario{Chave: "abc", TrechosIDs: []string{c.ab.Trechos[0].Trecho.ID, c.bc.Trechos[0].Trecho.ID}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// Viagens podem durar mais que a sessao: autentica novamente apos avancar o relogio.
// Recebe: t *testing.T.
// Retorna: nenhum valor; executa os efeitos descritos acima.
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
// Recebe: tipo string.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
func (c *cenarioViagem) cancelarConexao(tipo string) error {
	if tipo == "trecho" {
		_, err := c.g.CancelarTrecho(c.m2, c.bc.Trechos[0].Trecho.ID)
		return err
	}
	_, err := c.g.CancelarCarona(c.m2, c.bc.ID)
	return err
}

// TestCadastroAutenticacaoAutorizacaoEDesconexao: recebe o teste e cria uma conta normalizada.
// Confere senha, duplicacao, perfis, sessao desconhecida e revogacao no logout. Sem retorno.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestCadastroAutenticacaoAutorizacaoEDesconexao(t *testing.T) {
	g := NovoGrafo()
	g.agora = func() time.Time {
		return time.Date(2099, 10, 1, 8, 0, 0, 0, time.UTC)
	}
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

// TestBuscarValidaParametrosAntesDeExplorarOGrafo verifica o cenario: Buscar Valida Parametros Antes De Explorar OGrafo.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
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

// TestCancelarReservaIniciada: recebe o teste e avanca o relogio ate a partida da reserva.
// Confere que o cancelamento e recusado e a reserva permanece ativa. Sem retorno.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestCancelarReservaIniciada(t *testing.T) {
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
}

// TestPrecoPorTrechoValidacaoESoma verifica o cenario: Preco Por Trecho Validacao ESoma.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
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

// TestSessaoQuinzeMinutosELogsSemSegredos: recebe o teste e avanca o relogio da sessao.
// Confere consultas antes, no instante e depois da expiracao, limpeza e logs. Sem retorno.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestSessaoQuinzeMinutosELogsSemSegredos(t *testing.T) {
	c := criarCenario(t)
	var saida bytes.Buffer
	c.g.logger = log.New(&saida, "", 0)
	s, err := c.g.Autenticar(protocolo.Credenciais{Email: "p@teste.com", Senha: "senha1234"})
	if err != nil {
		t.Fatal(err)
	}
	expira, err := time.Parse(time.RFC3339, s.ExpiraEm)
	if err != nil || expira.Sub(c.agora) != 15*time.Minute {
		t.Fatalf("validade: %s %v", s.ExpiraEm, err)
	}
	if err := c.g.Desconectar(c.outro); err != nil {
		t.Fatal(err)
	}
	c.agora = expira.Add(-time.Second)
	if _, err := c.g.ConsultarReservas(s.ID); err != nil {
		t.Fatal(err)
	}
	c.agora = expira
	if _, err := c.g.ConsultarReservas(s.ID); err == nil {
		t.Fatal("sessão aceita no limite de expiração")
	}
	c.agora = expira.Add(time.Second)
	if _, err := c.g.ConsultarReservas(s.ID); err == nil {
		t.Fatal("sessão expirada aceita")
	}
	c.g.limparSessoesExpiradas()
	tamanho := saida.Len()
	c.g.limparSessoesExpiradas()
	if saida.Len() != tamanho {
		t.Fatal("expiração registrada novamente")
	}
	for _, esperado := range []string{"CONECTOU usuário=p@teste.com", "DESCONECTOU usuário=outro@teste.com", "DESCONECTOU usuário=p@teste.com motivo=sessão expirada (15 minutos)"} {
		if !strings.Contains(saida.String(), esperado) {
			t.Fatalf("log ausente %q: %s", esperado, &saida)
		}
	}
	if strings.Contains(saida.String(), s.ID) || strings.Contains(saida.String(), "senha1234") {
		t.Fatal("segredo no log")
	}
	if len(c.g.sessoes) != 0 {
		t.Fatal("sessões expiradas mantidas")
	}
}
