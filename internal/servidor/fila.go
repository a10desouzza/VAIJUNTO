// Arquivo: internal/servidor/fila.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Separa mensagens TCP completas em filas limitadas de leitura e escrita.
// Um trabalhador alterna a preferência entre as filas e devolve a resposta de cada pedido.
// Fila cheia ou encerramento geram respostas próprias para orientar o cliente.

package servidor

import (
	"context"

	"vaijunto/internal/protocolo"
)

type requisicaoNaFila struct {
	mensagem []byte
	resposta chan []byte
}

// As duas filas compartilham um trabalhador. Alternar a preferencia impede que
// consultas continuas deixem as escritas esperando, ou vice-versa.
type filaRequisicoes struct {
	leituras chan requisicaoNaFila
	escritas chan requisicaoNaFila
}

// novaFila cria duas filas com capacidade de 128 mensagens cada.
// Recebe: nenhum parametro.
// Retorna: um ponteiro para as filas de leitura e escrita inicializadas, cada uma com capacidade 128.
func novaFila() *filaRequisicoes {
	return &filaRequisicoes{
		leituras: make(chan requisicaoNaFila, 128),
		escritas: make(chan requisicaoNaFila, 128),
	}
}

// executar enfileira uma mensagem completa e espera apenas a resposta dela.
// Recebe: ctx context.Context, mensagem []byte.
// Retorna: []byte.
func (f *filaRequisicoes) executar(ctx context.Context, mensagem []byte) []byte {
	fila := f.escritas
	if mensagemDeConsulta(mensagem) {
		fila = f.leituras
	}
	pedido := requisicaoNaFila{mensagem: mensagem, resposta: make(chan []byte, 1)}
	select {
	case <-ctx.Done():
		return respostaOcupado("Servidor encerrando.")
	case fila <- pedido:
	default:
		return respostaOcupado("Fila cheia; tente novamente com a mesma chave.")
	}
	select {
	case resposta := <-pedido.resposta:
		return resposta
	case <-ctx.Done():
		return respostaOcupado("Servidor encerrando; consulte o resultado ao reconectar.")
	}
}

// atender e o unico trabalhador das mensagens TCP, alternando leituras e escritas.
// Recebe: ctx context.Context, g *GrafoItinerarios.
// Retorna: nenhum valor; executa os efeitos descritos acima.
func (f *filaRequisicoes) atender(ctx context.Context, g *GrafoItinerarios) {
	preferida, outra := f.leituras, f.escritas
	for {
		if ctx.Err() != nil {
			return
		}
		var pedido requisicaoNaFila
		select {
		case pedido = <-preferida:
		default:
			select {
			case <-ctx.Done():
				return
			case pedido = <-preferida:
			case pedido = <-outra:
			}
		}
		g.limparSessoesExpiradas()
		pedido.resposta <- ProcessarMensagem(pedido.mensagem, g)
		preferida, outra = outra, preferida
	}
}

// respostaOcupado codifica uma resposta de erro para sobrecarga ou encerramento do servidor.
// Recebe: mensagem string.
// Retorna: []byte.
func respostaOcupado(mensagem string) []byte {
	return codificarResposta(protocolo.Resposta{Status: protocolo.Erro, Mensagem: mensagem})
}
