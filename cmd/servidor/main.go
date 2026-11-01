// Arquivo: cmd/servidor/main.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Inicia uma empresa do projeto Consórcios com identidade e endereços próprios.
// Carrega o backup antes de abrir as portas TCP e REST e inicia a recuperação.
// Também coordena o encerramento das escutas quando o processo é interrompido.

package main

import (
	"context"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"vaijunto/internal/configuracao"
	"vaijunto/internal/consorcio"
	"vaijunto/internal/servidor"
)

// main inicia o estado local da empresa e, opcionalmente, sua API REST.
// Ctrl+C encerra os dois transportes.
// Recebe: nenhum parametro.
// Retorna: nenhum valor; executa os efeitos descritos acima.
func main() {
	padrao := os.Getenv("VAIJUNTO_ENDERECO")
	if padrao == "" {
		padrao = configuracao.EscutaPadrao
	}
	endereco := flag.String("endereco", padrao, "Endereço TCP de escuta (:0 escolhe uma porta livre)")
	timeout := flag.Duration("timeout", 30*time.Second, "Prazo de leitura e escrita por mensagem")
	empresa := flag.String("empresa", ambiente("VAIJUNTO_EMPRESA", "empresa-a"), "Identificador da empresa")
	rest := flag.String("rest", os.Getenv("VAIJUNTO_REST"), "Endereço REST; :0 escolhe porta livre, vazio desativa; configure os pares com a porta efetiva")
	pares := flag.String("pares", os.Getenv("VAIJUNTO_PARES"), "Empresas: empresa-b=http://host:porta,empresa-c=http://host:porta")
	timeoutREST := flag.Duration("timeout-rest", 3*time.Second, "Prazo de consulta a outra empresa")
	backup := flag.String("backup", ambiente("VAIJUNTO_BACKUP", "auto"), "Arquivo local de recuperacao; auto usa dados/EMPRESA.json; vazio desativa")
	flag.Parse()
	if *backup == "auto" {
		*backup = filepath.Join("dados", *empresa+".json")
	}
	if *timeout <= 0 {
		log.Fatal("timeout deve ser positivo")
	}
	empresas, err := configuracao.Empresas(*pares, *empresa)
	if err != nil {
		log.Fatal(err)
	}
	grafo := servidor.NovoGrafo()
	var api *consorcio.API
	var distribuido *servidor.Distribuido
	if *rest != "" {
		api, err = consorcio.NovaAPI(*empresa, os.Getenv("VAIJUNTO_TOKEN_CONSORCIO"), empresas, grafo, *timeoutREST)
		if err != nil {
			log.Fatal(err)
		}
	} else if len(empresas) > 0 {
		log.Fatal("configure -rest para participar do consórcio")
	}
	if len(empresas) > 0 {
		distribuido, err = servidor.NovoDistribuido(grafo, *empresa, os.Getenv("VAIJUNTO_TOKEN_CONSORCIO"), empresas, api, *timeoutREST)
		if err != nil {
			log.Fatal(err)
		}
	}
	if err := grafo.AbrirBackup(*empresa, *backup); err != nil {
		log.Fatal(err)
	}
	listener, err := net.Listen("tcp", *endereco)
	if err != nil {
		log.Fatal(err)
	}
	ctx, parar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer parar()
	if api != nil {
		mux := http.NewServeMux()
		mux.Handle("/", api.Handler())
		if distribuido != nil {
			mux.Handle("/distribuido/", distribuido.Handler())
			go distribuido.Recuperar(ctx)
		}
		httpListener, err := net.Listen("tcp", *rest)
		if err != nil {
			listener.Close()
			log.Fatal(err)
		}
		httpServidor := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: *timeoutREST + 10*time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
		fimREST := make(chan struct{})
		go func() {
			defer close(fimREST)
			if err := httpServidor.Serve(httpListener); err != nil && err != http.ErrServerClosed {
				log.Printf("REST encerrado: %v", err)
				parar()
			}
		}()
		defer func() {
			ctxEncerrar, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelar()
			if err := httpServidor.Shutdown(ctxEncerrar); err != nil {
				httpServidor.Close()
			}
			<-fimREST
		}()
		log.Printf("empresa=%s REST escutando em %s", *empresa, httpListener.Addr())
	}
	log.Printf("empresa=%s TCP escutando em %s", *empresa, listener.Addr())
	if err := servidor.Servir(ctx, listener, grafo, *timeout); err != nil {
		log.Printf("TCP encerrado: %v", err)
	}
}

// ambiente consulta a variavel de ambiente recebida e usa o valor padrao se ela estiver vazia.
// Recebe: nome, padrao string.
// Retorna: string.
func ambiente(nome, padrao string) string {
	if valor := os.Getenv(nome); valor != "" {
		return valor
	}
	return padrao
}
