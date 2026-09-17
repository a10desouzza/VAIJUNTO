// internal/carga/carga.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Teste de carga por sockets reais. Prepara usuarios, dispara reservas concorrentes
// e confere a integridade dos dois trechos de motoristas diferentes.

package carga

import (
	"crypto/rand"   // usado para gerar valores aleatorios de email, evitando colisao de contas
	"encoding/hex"  // converte bytes em textos alearorios para email
	"encoding/json" // converte structs para JSON e vice-versa
	"fmt"
	"math" // usado para calcular percentil
	"sort" // utilizado para colocar valores em ordem e calcular percentil
	"sync"
	"time"
	"vaijunto/internal/cliente"
	"vaijunto/internal/protocolo"
)

// Cria um tipo que reune varios valores de interesse relacionados
type Metricas struct {
	Clientes           int     `json:"clientes"`                // quantidade de passageiros concorrentes
	VagasPorTrecho     int     `json:"vagas_por_trecho"`        // quantidade de vagas disponiveis por trecho
	Confirmadas        int     `json:"confirmadas"`             // quantidade de reservas confirmadas
	Recusadas          int     `json:"recusadas"`               // quantidade de reservas recusadas
	ErrosTransporte    int     `json:"erros_transporte"`        // quantidade de erros no transporte
	DuracaoMS          float64 `json:"duracao_ms"`              // duracao total da simulacao em milissegundos
	LatenciaMediaMS    float64 `json:"latencia_media_ms"`       // latencia media em milissegundos
	LatenciaP95MS      float64 `json:"latencia_p95_ms"`         // latencia ao percentil 95 em milissegundos
	RequisicoesSegundo float64 `json:"requisicoes_por_segundo"` // quantidade de requisicoes por segundo
	Integridade        bool    `json:"integridade"`             // indica se a simulacao passou nos testes de integridade
}

// chamar: envia a operacao TCP e converte os dados para alvo. Propaga falhas como erro. Retorna:
// nil na resposta de sucesso; error na serializacao, transporte, recusa ou conversao.
func chamar(endereco, acao, sessaoID string, dados any, alvo any) error {
	// Converte os dados para JSON, envia a requisicao e recebe a resposta
	raw, err := json.Marshal(dados)
	if err != nil { // se houver erro na conversao, retorna o erro
		return err
	}
	// cria e envia a requisicao para o servidor, recebendo a resposta
	resposta, err := cliente.EnviarRequisicao(endereco, protocolo.Requisicao{Acao: acao, SessaoID: sessaoID, Dados: raw})
	if err != nil {
		return err
	}
	if resposta.Status != protocolo.Sucesso { // se a resposta nao for de sucesso, retorna o erro com a mensagem do servidor
		return fmt.Errorf("%s: %s", acao, resposta.Mensagem)
	}
	if alvo == nil { // se nao houver alvo para os dados, retorna nil
		return nil
	}
	raw, err = json.Marshal(resposta.Dados) // converte os dados da resposta para JSON
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, alvo) // converte os dados da resposta JSON para struct e retorna o erro, se houver
}

// criarUsuario: cadastra uma conta de teste e recebe a sessao criada automaticamente. Retorna:
// Identificador da sessao e nil, ou error se o cadastro falhar.
func criarUsuario(endereco, email, perfil string) (string, error) {
	senha := "senha-carga-" + email // senha padrao para todos os usuarios de teste
	var sessao protocolo.Sessao     // cria uma vareavel para armazenar a sessao retornada pelo servidor
	// chama a funcao chamar para enviar a requisicao de cadastro e receber a sessao
	err := chamar(endereco, protocolo.AcaoCadastrar, "", protocolo.Cadastro{Nome: "Teste de carga", Email: email, Senha: senha, Perfil: perfil}, &sessao)
	return sessao.ID, err // Devolve o ID da sessao e o erro, se houver
}

// Executar: Cadastra dois motoristas, publica dois trechos conectados e autentica os passageiros.
// Libera as goroutines por um canal, mede as confirmacoes e verifica vagas e assentos unicos. Ao
// concluir a fase de uso, tenta cancelar as ofertas e encerrar as sessoes dos passageiros.
// Retorna: Metricas e nil se a integridade e o transporte passarem; metricas parciais e error na
// falha.
func Executar(endereco string, quantidade, vagas int) (Metricas, error) {
	//cria uma variavel m que guarda as metricas do teste
	m := Metricas{Clientes: quantidade, VagasPorTrecho: vagas}
	//valida os valores
	if quantidade < 1 || quantidade > 200 || vagas < 1 || vagas > 100 {
		return m, fmt.Errorf("clientes deve estar entre 1 e 200; vagas entre 1 e 100")
	}
	//cria estruturas de tamanho 8 bytes
	aleatorio := make([]byte, 8)
	//preenche a estrutura com bytes aleatorios
	if _, err := rand.Read(aleatorio); err != nil {
		return m, err
	}
	//transforma os bytes em texto hexadecimal
	prefixo := hex.EncodeToString(aleatorio)
	//cria arrays de motoristas e caronas com tamanho 2
	motoristas := make([]string, 2)
	caronas := make([]protocolo.Carona, 2)

	for i := range motoristas {
		// cria um motorista com email unico
		sessaoID, err := criarUsuario(endereco, fmt.Sprintf("m%d-%s@carga.test", i, prefixo), protocolo.Motorista)
		if err != nil {
			return m, err
		}
		// registra o ID da sessao do motorista no array de motoristas
		motoristas[i] = sessaoID
		// cria um array de cidades com prefixo unico
		cidades := []string{"Carga-" + prefixo + "-A", "Carga-" + prefixo + "-B", "Carga-" + prefixo + "-C"}
		// trecho com horaris diferentes para criar roteiros conectados
		hora := "2099-10-01T08:00:00-03:00"
		if i == 1 {
			hora = "2099-10-01T09:15:00-03:00"
		}
		// cria uma oferta de carona com os dados do motorista, cidades, hora e vagas e cria um trecho com preco, distancia e tempo de viagem
		oferta := protocolo.PublicacaoCarona{Chave: "carga", Rota: cidades[i : i+2], DataHora: hora, Assentos: vagas, Trechos: []protocolo.OfertaTrecho{{Preco: 10, DistanciaKM: 10, TempoViagem: 60}}}
		// publica carona no servidor e registra o ID da carona no array de caronas
		if err := chamar(endereco, protocolo.AcaoPublicar, sessaoID, oferta, &caronas[i]); err != nil {
			return m, err
		}
	}
	// garante que as caronas serao canceladas ao final da funcao, mesmo que ocorra um erro
	defer func() {
		//percorre o array de motoristas
		for i, sessaoID := range motoristas {
			// cancela a carona do motorista com o ID da carona correspondente
			chamar(endereco, protocolo.AcaoCancelarCarona, sessaoID, protocolo.Identificador{ID: caronas[i].ID}, nil)
		}
	}()
	// cria lista de sessoes, uma para cada passageiro
	sessoes := make([]string, quantidade)
	// repete para cada passageiro
	for i := range sessoes {
		// cria um email para o passageiro
		sessaoID, err := criarUsuario(endereco, fmt.Sprintf("p%d-%s@carga.test", i, prefixo), protocolo.Passageiro)
		if err != nil {
			return m, err
		}
		sessoes[i] = sessaoID //salva a sessao
	}
	// ao fim da funcao, encerra sessao de cada passageiro
	defer func() {
		for _, sessaoID := range sessoes {
			chamar(endereco, protocolo.AcaoSair, sessaoID, struct{}{}, nil)
		}
	}()
	//pega os IDs dos trechos das caronas publicadas
	ids := []string{caronas[0].Trechos[0].Trecho.ID, caronas[1].Trechos[0].Trecho.ID}
	//cria pedidos de resercar exatamente os dois trechos
	pedido, _ := json.Marshal(protocolo.ReservaItinerario{Chave: "unica", TrechosIDs: ids})
	type resultado struct { // para cada passageiro, guarda a resposta, o erro e a duracao da requisicao
		resposta *protocolo.Resposta
		err      error
		duracao  time.Duration
	}
	resultados := make([]resultado, quantidade) // cria uma lista com resultado para cada passageiro
	/* Barreira de inicio: todas as goroutines esperam o fechamento do mesmo canal. */
	inicio := make(chan struct{})
	var wg sync.WaitGroup //contador de tarefas
	// para cada passageiro incrementa o contador de tarefas
	for i, sessaoID := range sessoes {
		wg.Add(1)
		// adiciona uma tarefa para cada passageiro, que envia a requisicao de confirmacao e guarda o resultado
		go func() {
			defer wg.Done() //quando terminar, decremeta o contador
			<-inicio        // para aqui e espera o fechamento do canal de inicio
			/* A medicao comeca apos preparar usuarios e ofertas; cobre a disputa pelas reservas. */
			instante := time.Now()
			// o passageiro tenta confirmr a reserva
			resposta, err := cliente.EnviarRequisicao(endereco, protocolo.Requisicao{Acao: protocolo.AcaoConfirmar, SessaoID: sessaoID, Dados: pedido})
			/* Cada goroutine escreve em seu indice; a leitura so ocorre depois do Wait. */
			resultados[i] = resultado{resposta: resposta, err: err, duracao: time.Since(instante)}
		}()
	}
	// libera todas as goroutines para iniciar a disputa pelas reservas
	instante := time.Now()
	close(inicio) // fecha o canal de inicio, liberando todas as goroutines para reservar quase simultaneamente
	wg.Wait()
	duracao := time.Since(instante)
	latencias := make([]float64, quantidade)
	confirmadas := make(map[string]bool)
	integridade := true
	// percorre os resultados de cada passageiro, calcula latencias, conta confirmacoes e recusas, e verifica integridade
	for i, r := range resultados {
		latencias[i] = float64(r.duracao) / float64(time.Millisecond)
		m.LatenciaMediaMS += latencias[i] / float64(quantidade)
		if r.err != nil {
			m.ErrosTransporte++
			continue
		}
		if r.resposta.Status == protocolo.Erro {
			m.Recusadas++
			continue
		}
		m.Confirmadas++
		var reserva protocolo.Reserva
		raw, _ := json.Marshal(r.resposta.Dados)
		if err := json.Unmarshal(raw, &reserva); err != nil {
			integridade = false
			continue
		}
		if confirmadas[reserva.ID] || len(reserva.Assentos) != 2 || reserva.Status != protocolo.Ativa {
			integridade = false
		}
		confirmadas[reserva.ID] = true
		for j, a := range reserva.Assentos {
			if j >= len(ids) || a.Trecho.ID != ids[j] {
				integridade = false
			}
		}
	}
	/* P95: valor que cobre pelo menos 95% das latencias medidas, apos ordenacao. */
	sort.Float64s(latencias)
	m.LatenciaP95MS = latencias[int(math.Ceil(float64(quantidade)*0.95))-1]
	m.DuracaoMS = float64(duracao) / float64(time.Millisecond)
	m.RequisicoesSegundo = float64(quantidade) / duracao.Seconds()
	// Verifica integridade das caronas publicadas, vagas e assentos unicos
	for i, sessaoID := range motoristas {
		var publicadas []protocolo.Carona
		if err := chamar(endereco, protocolo.AcaoCaronas, sessaoID, struct{}{}, &publicadas); err != nil {
			return m, err
		}
		if len(publicadas) != 1 || len(publicadas[0].Trechos) != 1 {
			return m, fmt.Errorf("carona de carga ausente")
		}
		t := publicadas[0].Trechos[0]
		if t.Trecho.ID != ids[i] || t.Trecho.Assentos != vagas-m.Confirmadas || len(t.Passageiros) != m.Confirmadas {
			integridade = false
		}
		assentos := make(map[int]bool)
		for _, p := range t.Passageiros {
			if assentos[p.Assento] || p.Assento < 1 || p.Assento > vagas || !confirmadas[p.ReservaID] {
				integridade = false
			}
			assentos[p.Assento] = true
		}
	}
	esperado := min(vagas, quantidade)
	m.Integridade = integridade && m.ErrosTransporte == 0 && m.Confirmadas == esperado && m.Recusadas == quantidade-esperado
	if !m.Integridade {
		return m, fmt.Errorf("teste de carga falhou na integridade ou no transporte")
	}
	return m, nil
}
