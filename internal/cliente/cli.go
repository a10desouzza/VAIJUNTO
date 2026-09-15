package cliente

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"vaijunto/internal/protocolo"
)

func Executar(perfil string) error {
	padrao := os.Getenv("VAIJUNTO_SERVIDOR")
	if padrao == "" {
		padrao = "127.0.0.1:8080"
	}
	endereco := flag.String("servidor", padrao, "IP:porta do servidor TCP")
	acao := flag.String("acao", "", "CADASTRAR, AUTENTICAR, DESCONECTAR ou operação do perfil")
	token := flag.String("token", os.Getenv("VAIJUNTO_TOKEN"), "Token obtido em AUTENTICAR")
	dados := flag.String("dados", "{}", "Objeto JSON da operação; use - para ler da entrada padrão")
	arquivo := flag.String("arquivo", "", "Arquivo JSON com os dados da operação")
	flag.Parse()
	if *acao == "" {
		return ExecutarMenu(*endereco, perfil, os.Stdin, os.Stdout)
	}
	permitida := *acao == protocolo.AcaoCadastrar || *acao == protocolo.AcaoEntrar || *acao == protocolo.AcaoSair
	permitida = permitida || *acao == protocolo.AcaoNotificacoes || *acao == protocolo.AcaoLerNotificacao
	if perfil == protocolo.Motorista {
		permitida = permitida || *acao == protocolo.AcaoPublicar || *acao == protocolo.AcaoCaronas || *acao == protocolo.AcaoCancelarCarona || *acao == protocolo.AcaoCancelarTrecho
	}
	if perfil == protocolo.Passageiro {
		permitida = permitida || *acao == protocolo.AcaoBuscar || *acao == protocolo.AcaoConfirmar || *acao == protocolo.AcaoReservas || *acao == protocolo.AcaoCancelarReserva
	}
	if !permitida {
		return fmt.Errorf("ação indisponível neste cliente")
	}
	raw := []byte(*dados)
	if *arquivo != "" || *dados == "-" {
		var leitor io.Reader = os.Stdin
		if *arquivo != "" {
			f, err := os.Open(*arquivo)
			if err != nil {
				return err
			}
			defer f.Close()
			leitor = f
		}
		var err error
		raw, err = io.ReadAll(io.LimitReader(leitor, protocolo.LimiteMensagem))
		if err != nil {
			return err
		}
	}
	if !json.Valid(raw) {
		return fmt.Errorf("dados devem ser JSON válido")
	}
	if *acao == protocolo.AcaoCadastrar {
		var objeto map[string]json.RawMessage
		if err := json.Unmarshal(raw, &objeto); err != nil || objeto == nil {
			return fmt.Errorf("cadastro deve ser objeto JSON")
		}
		if _, ok := objeto["perfil"]; !ok {
			objeto["perfil"], _ = json.Marshal(perfil)
		}
		var err error
		raw, err = json.Marshal(objeto)
		if err != nil {
			return err
		}
	}
	resp, err := EnviarRequisicao(*endereco, protocolo.Requisicao{Acao: *acao, Token: *token, Dados: raw})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(resp); err != nil {
		return err
	}
	if resp.Status == protocolo.Erro {
		return fmt.Errorf("operação recusada")
	}
	return nil
}
