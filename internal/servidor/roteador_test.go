package servidor

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"vaijunto/internal/protocolo"
)

func TestDecodificarAceitaObjetoEstrito(t *testing.T) {
	var destino struct {
		Nome string `json:"nome"`
	}
	if err := decodificar([]byte(`{"nome":"Ana"}`), &destino); err != nil {
		t.Fatal(err)
	}
	if destino.Nome != "Ana" {
		t.Fatalf("nome = %q", destino.Nome)
	}
}

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

func TestProcessarMensagemRespeitaLimite(t *testing.T) {
	raw := ProcessarMensagem(bytes.Repeat([]byte{'x'}, protocolo.LimiteMensagem), NovoGrafo())
	var resposta protocolo.Resposta
	if err := json.Unmarshal(raw, &resposta); err != nil {
		t.Fatal(err)
	}
	if resposta.Status != protocolo.Erro || !strings.Contains(resposta.Mensagem, "limite") {
		t.Fatalf("resposta = %+v", resposta)
	}
}

func TestCadastroPeloRoteadorCriaSessaoAutomatica(t *testing.T) {
	dados, err := json.Marshal(protocolo.Cadastro{
		Nome: "Ana Teste", Email: "ana@cadastro.test", Senha: "senha1234", Perfil: protocolo.Passageiro,
	})
	if err != nil {
		t.Fatal(err)
	}
	mensagem, err := json.Marshal(protocolo.Requisicao{Acao: protocolo.AcaoCadastrar, Dados: dados})
	if err != nil {
		t.Fatal(err)
	}
	var resposta protocolo.Resposta
	g := NovoGrafo()
	if err := json.Unmarshal(ProcessarMensagem(mensagem, g), &resposta); err != nil {
		t.Fatal(err)
	}
	if resposta.Status != protocolo.Sucesso {
		t.Fatalf("cadastro recusado: %+v", resposta)
	}
	raw, err := json.Marshal(resposta.Dados)
	if err != nil {
		t.Fatal(err)
	}
	var sessao protocolo.Sessao
	if err := json.Unmarshal(raw, &sessao); err != nil {
		t.Fatal(err)
	}
	if sessao.ID == "" || sessao.Usuario.Email != "ana@cadastro.test" || sessao.Usuario.Perfil != protocolo.Passageiro {
		t.Fatalf("sessão automática inválida: %+v", sessao)
	}
	if _, err := g.ConsultarReservas(sessao.ID); err != nil {
		t.Fatalf("sessão criada no cadastro não autorizou a consulta: %v", err)
	}
}

func TestCodificarRespostaTemQuebraDeLinhaEFallback(t *testing.T) {
	normal := codificarResposta(protocolo.Resposta{Status: protocolo.Sucesso, Mensagem: "ok"})
	if normal[len(normal)-1] != '\n' || !json.Valid(bytes.TrimSpace(normal)) {
		t.Fatalf("resposta normal inválida: %q", normal)
	}

	fallback := codificarResposta(protocolo.Resposta{Status: protocolo.Sucesso, Dados: math.Inf(1)})
	if string(fallback) != "{\"status\":\"ERRO\",\"mensagem\":\"Falha ao codificar resposta.\"}\n" {
		t.Fatalf("fallback inesperado: %q", fallback)
	}
}
