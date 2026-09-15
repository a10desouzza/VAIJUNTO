package cliente_test

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"
	"time"
	"vaijunto/internal/cliente"
	"vaijunto/internal/protocolo"
	"vaijunto/internal/servidor"
)

func TestMenusFluxoCompletoTCP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancelar := context.WithCancel(context.Background())
	fim := make(chan error, 1)
	go func() { fim <- servidor.Servir(ctx, listener, servidor.NovoGrafo(), time.Second) }()
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
	executar(protocolo.Motorista, "2\nAna\nana@menu.test\nsenha1234\nsenha1234\n1\nana@menu.test\nsenha1234\n1\n3\nSalvador\nFeira\nSerrinha\n2099-10-01\n08:00\n3\n1\n45,50\n120\n15\n25\n60\n0\n1\n2\n0\n", "Conta criada", "publicada com sucesso", "3/3 vagas")
	executar(protocolo.Passageiro, "2\nBia\nbia@menu.test\nsenha1234\nsenha1234\n1\nbia@menu.test\nsenha1234\n1\nSalvador\nSerrinha\n2099-10-01\n1\n1\n1\n2\n0\n", "confirmada!", "R$ 70,50", "1 vaga reservada", "ATIVA")
	executar(protocolo.Motorista, "1\nana@menu.test\nsenha1234\n2\n0\n", "2/3 vagas", "Bia")
	executar(protocolo.Passageiro, "1\nbia@menu.test\nsenha1234\n3\n1\n1\n2\n0\n", "Os assentos foram devolvidos", "CANCELADA")
	executar(protocolo.Passageiro, "1\nbia@menu.test\nsenha1234\n1\nSalvador\nSerrinha\n2099-10-01\n1\n1\n1\n0\n", "confirmada!")
	executar(protocolo.Motorista, "1\nana@menu.test\nsenha1234\n5\n1\n2\n1\n2\n0\n", "Trecho cancelado", "PARCIALMENTE_CANCELADA", "3/3 vagas")
	executar(protocolo.Passageiro, "1\nbia@menu.test\nsenha1234\n5\n2\n0\n", "Aviso da reserva", "itinerário inteiro cancelado", "CANCELADA")
	executar(protocolo.Motorista, "1\nana@menu.test\nsenha1234\n2\n3\n1\n1\n2\n0\n", "3/3 vagas", "Carona cancelada", "CANCELADA")
}

func TestMenuEntradaInvalidaEEncerramento(t *testing.T) {
	for _, entrada := range []string{"", "abc\n9\n0\n", "2\nNome\n"} {
		var saida bytes.Buffer
		if err := cliente.ExecutarMenu("127.0.0.1:1", protocolo.Passageiro, strings.NewReader(entrada), &saida); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(saida.String(), "Cliente encerrado") {
			t.Fatal(saida.String())
		}
	}
}
