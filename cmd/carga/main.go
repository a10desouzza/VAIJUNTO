package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"vaijunto/internal/carga"
)

func main() {
	padrao := os.Getenv("VAIJUNTO_SERVIDOR")
	if padrao == "" {
		padrao = "127.0.0.1:8080"
	}
	servidor := flag.String("servidor", padrao, "IP:porta do servidor TCP")
	clientes := flag.Int("clientes", 100, "Clientes concorrentes, de 1 a 200")
	vagas := flag.Int("vagas", 5, "Vagas em cada um dos dois trechos")
	flag.Parse()
	metricas, err := carga.Executar(*servidor, *clientes, *vagas)
	json.NewEncoder(os.Stdout).Encode(metricas)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
