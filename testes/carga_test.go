// testes/carga_test.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Teste de carga automatizado contra um servidor TCP local.

package testes

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"
	"vaijunto/internal/carga"
	"vaijunto/internal/servidor"
)

// TestCargaTCP: Inicia um servidor TCP local e executa a carga com 50 clientes e tres vagas,
// registrando as metricas.
func TestCargaTCP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancelar := context.WithCancel(context.Background())
	fim := make(chan error, 1)
	go func() { fim <- servidor.Servir(ctx, listener, servidor.NovoGrafo(), 5*time.Second) }()
	defer func() {
		cancelar()
		if err := <-fim; err != nil {
			t.Error(err)
		}
	}()
	metricas, err := carga.Executar(listener.Addr().String(), 50, 3)
	raw, _ := json.Marshal(metricas)
	t.Log(string(raw))
	if err != nil {
		t.Fatal(err)
	}
}
