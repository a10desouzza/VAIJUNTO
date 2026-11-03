// Autor: Arthur de Andrade Souza.
// Matéria: TEC502 - TP01 - Concorrência e Conectividade - 2026.2.
// Declaração de não plágio: Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
// Funcionalidade: Prepara cenários locais e testa autenticação, autorização, preços, sessões e validação JSON.
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

// criarCenario prepara usuários e ofertas conectadas com horário controlado para os testes locais.
// Recebe: controle de teste t.
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

// reservar confirma a reserva usada pelo cenário de teste.
// Recebe: cenário c e controle t.
func (c *cenarioViagem) reservar(t *testing.T) protocolo.Reserva {
	t.Helper()
	r, err := c.g.Confirmar(c.p, protocolo.ReservaItinerario{Chave: "abc", TrechosIDs: []string{c.ab.Trechos[0].Trecho.ID, c.bc.Trechos[0].Trecho.ID}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// reautenticar renova as sessões do cenário após mudanças no horário controlado.
// Recebe: cenário c e controle t.
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

// cancelarConexao tenta cancelar a conexão por trecho ou carona no cenário local.
// Recebe: tipo de cancelamento e cenário c.
func (c *cenarioViagem) cancelarConexao(tipo string) error {
	if tipo == "trecho" {
		_, err := c.g.CancelarTrecho(c.m2, c.bc.Trechos[0].Trecho.ID)
		return err
	}
	_, err := c.g.CancelarCarona(c.m2, c.bc.ID)
	return err
}

// TestCadastroAutenticacaoAutorizacaoEDesconexao verifica contas, credenciais, restrições de perfil e encerramento de sessão.
// Recebe: controle de teste t.
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

// TestPrecoPorTrechoValidacaoESoma verifica validação dos preços e soma monetária por trecho.
// Recebe: controle de teste t.
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

// TestSessaoQuinzeMinutosELogsSemSegredos verifica expiração da sessão e ausência de credenciais nos registros de log.
// Recebe: controle de teste t.
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

// TestDecodificarRejeitaJSONAmbiguoOuForaDoContrato verifica rejeição de JSON ambíguo, campos desconhecidos e formatos inválidos.
// Recebe: controle de teste t.
func TestDecodificarRejeitaJSONAmbiguoOuForaDoContrato(t *testing.T) {
	casos := map[string][]byte{
		"vazio":              nil,
		"escalar":            []byte(`42`),
		"lista":              []byte(`[]`),
		"utf8 inválido":      {0xff, '{', '}'},
		"campo desconhecido": []byte(`{"nome":"Ana","extra":true}`),
		"campo duplicado":    []byte(`{"nome":"Ana","nome":"Bia"}`),
		"duplicado aninhado": []byte(`{"nome":"Ana","obj":{"x":1,"x":2}}`),
		"dois objetos":       []byte(`{"nome":"Ana"} {"nome":"Bia"}`),
	}
	for nome, dados := range casos {
		t.Run(nome, func(t *testing.T) {
			var destino struct {
				Nome string `json:"nome"`
			}
			if err := decodificar(dados, &destino); err == nil {
				t.Fatal("entrada inválida foi aceita")
			}
		})
	}
}
