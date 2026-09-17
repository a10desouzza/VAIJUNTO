// testes/ambiente_test.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Preparacao compartilhada de contas e caronas para testes com estados independentes.

package testes

import (
	"testing"
	"vaijunto/internal/protocolo"
	"vaijunto/internal/servidor"
)

type ambiente struct {
	g               *servidor.GrafoItinerarios
	motorista       string
	outroMotorista  string
	passageiro      string
	outroPassageiro string
}

// usuario: Cadastra uma conta de teste e autentica para que os cenarios possam chamar operacoes
// protegidas. Retorna: Token autenticado. Interrompe o teste com Fatal se nao conseguir preparar a
// conta.
func usuario(t testing.TB, g *servidor.GrafoItinerarios, email, perfil string) string {
	t.Helper()
	if _, err := g.Cadastrar(protocolo.Cadastro{Nome: "Pessoa Teste", Email: email, Senha: "senha-teste-123", Perfil: perfil}); err != nil {
		t.Fatal(err)
	}
	s, err := g.Autenticar(protocolo.Credenciais{Email: email, Senha: "senha-teste-123"})
	if err != nil {
		t.Fatal(err)
	}
	return s.ID
}

// preparar: Isola os dados de cada teste e prepara contas com perfis diferentes. Retorna: Ambiente
// com grafo novo e sessoes de dois motoristas e dois passageiros.
func preparar(t testing.TB) ambiente {
	t.Helper()
	g := servidor.NovoGrafo()
	return ambiente{g: g, motorista: usuario(t, g, "m@teste.com", protocolo.Motorista), outroMotorista: usuario(t, g, "m2@teste.com", protocolo.Motorista), passageiro: usuario(t, g, "p@teste.com", protocolo.Passageiro), outroPassageiro: usuario(t, g, "p2@teste.com", protocolo.Passageiro)}
}

// oferta: Monta dados padrao de distancia e tempos para reutilizar nos cenarios. Retorna:
// PublicacaoCarona com uma oferta por par de cidades consecutivas.
func oferta(chave string, rota []string, hora string, vagas int) protocolo.PublicacaoCarona {
	trechos := make([]protocolo.OfertaTrecho, len(rota)-1)
	for i := range trechos {
		trechos[i] = protocolo.OfertaTrecho{Preco: 10, DistanciaKM: 10, TempoViagem: 60, TempoParada: 15}
	}
	return protocolo.PublicacaoCarona{Chave: chave, Rota: rota, DataHora: hora, Assentos: vagas, Trechos: trechos}
}

// publicar: Usa a funcao oferta e publica no estado central do cenario. Retorna: Carona publicada.
// Interrompe o teste com Fatal se a preparacao falhar.
func publicar(t testing.TB, a ambiente, sessaoID, chave string, rota []string, hora string, vagas int) protocolo.Carona {
	t.Helper()
	c, err := a.g.Publicar(sessaoID, oferta(chave, rota, hora, vagas))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// ids: Extrai os IDs para montar uma requisicao de reserva. Retorna: Lista com IDs de seus trechos
// na mesma ordem.
func ids(c protocolo.Carona) []string {
	r := make([]string, len(c.Trechos))
	for i, t := range c.Trechos {
		r[i] = t.Trecho.ID
	}
	return r
}
