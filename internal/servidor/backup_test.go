// Arquivo: internal/servidor/backup_test.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Testa a gravação e a restauração do estado privado da empresa.
// Verifica recuperação nas fases preparada, decidida e confirmada e preservação das chaves.
// Também cobre arquivo corrompido e falha de gravação antes da resposta de sucesso.

package servidor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"vaijunto/internal/configuracao"
	"vaijunto/internal/consorcio"
	"vaijunto/internal/protocolo"
)

// TestBackupRecuperaReservaLocalEChave verifica o cenario: Backup Recupera Reserva Local EChave.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestBackupRecuperaReservaLocalEChave(t *testing.T) {
	c := criarCenario(t)
	caminho := filepath.Join(t.TempDir(), "empresa.json")
	if err := c.g.AbrirBackup("local", caminho); err != nil {
		t.Fatal(err)
	}
	pedido := protocolo.ReservaItinerario{Chave: "backup", TrechosIDs: []string{c.ab.Trechos[0].Trecho.ID}}
	dados, _ := json.Marshal(pedido)
	mensagem, _ := json.Marshal(protocolo.Requisicao{Acao: protocolo.AcaoConfirmar, SessaoID: c.p, Dados: dados})
	var resposta protocolo.Resposta
	if err := json.Unmarshal(ProcessarMensagem(mensagem, c.g), &resposta); err != nil || resposta.Status != protocolo.Sucesso {
		t.Fatalf("confirmacao: %+v %v", resposta, err)
	}
	novo := NovoGrafo()
	if err := novo.AbrirBackup("local", caminho); err != nil {
		t.Fatal(err)
	}
	if _, err := novo.ConsultarReservas(c.p); err == nil {
		t.Fatal("sessao antiga sobreviveu ao reinicio")
	}
	s, err := novo.Autenticar(protocolo.Credenciais{Email: "p@teste.com", Senha: "senha1234"})
	if err != nil {
		t.Fatal("conta nao recuperada", err)
	}
	reservas, err := novo.ConsultarReservas(s.ID)
	if err != nil || len(reservas) != 1 || reservas[0].Status != protocolo.Ativa {
		t.Fatalf("reserva nao recuperada: %+v %v", reservas, err)
	}
	repetida, err := novo.Confirmar(s.ID, pedido)
	if err != nil || repetida.ID != reservas[0].ID || novo.trechos[pedido.TrechosIDs[0]].trecho.Assentos != 1 {
		t.Fatalf("repeticao descontou outra vaga: %+v %v", repetida, err)
	}
}

// TestBackupDanificadoNaoEApagado verifica o cenario: Backup Danificado Nao EApagado.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestBackupDanificadoNaoEApagado(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "empresa.json")
	conteudo := []byte(`{"versao":1,"estado":`)
	if err := os.WriteFile(caminho, conteudo, 0600); err != nil {
		t.Fatal(err)
	}
	if err := NovoGrafo().AbrirBackup("local", caminho); err == nil {
		t.Fatal("backup truncado foi aceito")
	}
	depois, err := os.ReadFile(caminho)
	if err != nil || !bytes.Equal(conteudo, depois) {
		t.Fatal("arquivo danificado foi substituido")
	}
}

// TestBackupNaoRespondeSucessoQuandoDiscoFalha verifica o cenario: Backup Nao Responde Sucesso Quando Disco Falha.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestBackupNaoRespondeSucessoQuandoDiscoFalha(t *testing.T) {
	g := NovoGrafo()
	pasta := t.TempDir()
	if err := g.AbrirBackup("local", filepath.Join(pasta, "estado.json")); err != nil {
		t.Fatal(err)
	}
	// Um diretorio como destino impede a substituicao do arquivo em qualquer SO.
	g.backup.caminho = pasta
	mensagem := []byte(`{"acao":"CADASTRAR","dados":{"nome":"Pessoa Teste","email":"disco@teste.com","senha":"senha1234","perfil":"PASSAGEIRO"}}`)
	var resposta protocolo.Resposta
	if err := json.Unmarshal(ProcessarMensagem(mensagem, g), &resposta); err != nil || resposta.Status != protocolo.Erro {
		t.Fatalf("escrita sem backup recebeu sucesso: %+v %v", resposta, err)
	}
}

// TestBackupRecuperaConsorcioAposReinicio verifica o cenario: Backup Recupera Consorcio Apos Reinicio.
// Recebe: t *testing.T.
// Retorna: nao possui retorno; registra falhas no objeto de teste quando o resultado viola o esperado.
func TestBackupRecuperaConsorcioAposReinicio(t *testing.T) {
	for _, fase := range []string{"preparada", "escolhida", "confirmada"} {
		t.Run(fase, func(t *testing.T) {
			empresas := criarConsorcio(t, 1)
			caminhos := make([]string, len(empresas))
			geracoes := make([]string, len(empresas))
			for i, e := range empresas {
				caminhos[i] = filepath.Join(t.TempDir(), e.d.id+".json")
				geracoes[i] = e.d.geracao
				if err := e.g.AbrirBackup(e.d.id, caminhos[i]); err != nil {
					t.Fatal(err)
				}
			}
			a := empresas[0]
			interromper := func() error {
				return fmt.Errorf("processo interrompido")
			}
			if fase == "preparada" {
				a.d.aposPreparar = interromper
			} else if fase == "escolhida" {
				a.d.aposQuorum = interromper
			}
			pedido := pedidoTeste(empresas, "reiniciar")
			_, err := a.d.reservar(a.passageiro, pedido)
			if fase == "confirmada" && err != nil {
				t.Fatal(err)
			}
			// Descarta todos os grafos e protocolos. Os novos processos recebem somente
			// a configuracao e o arquivo privado da respectiva empresa.
			for _, e := range empresas {
				e.http.Close()
			}
			for i, e := range empresas {
				reiniciarEmpresaComBackup(t, e, caminhos[i])
				if e.d.geracao != geracoes[i] || len(e.g.usuarios) != 2 {
					t.Fatal("identidade ou dados locais nao foram preservados")
				}
			}
			a.d.abortarApos = 0
			ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancelar()
			a.d.recuperar(ctx)
			if fase == "preparada" {
				verificarVagas(t, empresas, 1, 0, 0)
			} else {
				verificarVagas(t, empresas, 0, 1, 0)
			}
			r, err := a.d.reservar(a.passageiro, pedido)
			if err != nil {
				t.Fatal("repeticao apos recuperar", err)
			}
			if fase == "preparada" {
				if r.Status != protocolo.Cancelada {
					t.Fatal("preparacao sem decisao nao foi abortada")
				}
			} else {
				if r.Status != protocolo.Ativa {
					t.Fatal("decisao escolhida foi perdida")
				}
				if _, err := a.d.cancelar(a.passageiro, r.ID); err != nil {
					t.Fatal("cancelamento apos recuperar", err)
				}
				verificarVagas(t, empresas, 1, 0, 0)
			}
		})
	}
}

// reiniciarEmpresaComBackup recria grafo, protocolo e HTTP no endereco anterior restaurando somente o backup privado.
// Recebe: t *testing.T, e *empresaTeste, caminho string.
// Retorna: nenhum valor; executa os efeitos descritos acima.
func reiniciarEmpresaComBackup(t *testing.T, e *empresaTeste, caminho string) {
	t.Helper()
	endereco := e.http.Listener.Addr().String()
	antigo := e.d
	pares := make([]configuracao.Empresa, 0, 2)
	for id, url := range antigo.pares {
		pares = append(pares, configuracao.Empresa{ID: id, URL: url})
	}
	g := NovoGrafo()
	api, err := consorcio.NovaAPI(antigo.id, antigo.token, pares, g, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	d, err := NovoDistribuido(g, antigo.id, antigo.token, pares, api, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.AbrirBackup(d.id, caminho); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/", api.Handler())
	mux.Handle("/distribuido/", d.Handler())
	httpServidor := httptest.NewUnstartedServer(mux)
	httpServidor.Listener.Close()
	httpServidor.Listener, err = net.Listen("tcp", endereco)
	if err != nil {
		t.Fatal(err)
	}
	httpServidor.Start()
	t.Cleanup(httpServidor.Close)
	e.g, e.d, e.http = g, d, httpServidor
	for _, perfil := range []string{protocolo.Passageiro, protocolo.Motorista} {
		for email, c := range g.usuarios {
			if c.usuario.Perfil == perfil {
				s, err := g.Autenticar(protocolo.Credenciais{Email: email, Senha: "senha1234"})
				if err != nil {
					t.Fatal(err)
				}
				if perfil == protocolo.Motorista {
					e.motorista = s.ID
				} else {
					e.passageiro = s.ID
				}
			}
		}
	}
}
