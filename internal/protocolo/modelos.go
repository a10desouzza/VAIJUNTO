/* ================================================================================================
 * internal/protocolo/modelos.go - VaiJunto: sistema de caronas compartilhadas
 * Autor: Arthur Souza
 *
 * Estruturas compartilhadas pelos clientes e servidor.
 * As tags json definem os nomes dos campos na rede. O transporte usa texto JSON, nao memoria bruta.
 *
 * DIVISAO DE RESPONSABILIDADES:
 * Este pacote define os dados compartilhados. Os sockets e as regras de reserva ficam nos pacotes
 * cliente e servidor.
 * ================================================================================================ */

package protocolo

import "encoding/json"

const (
	AcaoCadastrar       = "CADASTRAR"
	AcaoEntrar          = "AUTENTICAR"
	AcaoSair            = "DESCONECTAR"
	AcaoPublicar        = "PUBLICAR_CARONA"
	AcaoBuscar          = "BUSCAR_ITINERARIO"
	AcaoConfirmar       = "CONFIRMAR_RESERVA"
	AcaoCaronas         = "CONSULTAR_CARONAS"
	AcaoCancelarCarona  = "CANCELAR_CARONA"
	AcaoReservas        = "CONSULTAR_RESERVAS"
	AcaoCancelarReserva = "CANCELAR_RESERVA"
	AcaoCancelarTrecho  = "CANCELAR_TRECHO"
	AcaoNotificacoes    = "CONSULTAR_NOTIFICACOES"
	AcaoLerNotificacao  = "LER_NOTIFICACAO"
	Parcial             = "PARCIALMENTE_CANCELADA"
	Sucesso             = "SUCESSO"
	Erro                = "ERRO"
	Ativa               = "ATIVA"
	Cancelada           = "CANCELADA"
	Motorista           = "MOTORISTA"
	Passageiro          = "PASSAGEIRO"
	LimiteMensagem      = 1 << 20
)

/* Requisicao: Envelope recebido. RawMessage adia a leitura de dados ate o roteador conhecer a acao. */
type Requisicao struct {
	Acao  string          `json:"acao"`
	Token string          `json:"token,omitempty"`
	Dados json.RawMessage `json:"dados"`
}

/* Resposta: Envelope devolvido ao cliente. dados e omitido quando nao ha conteudo para retornar. */
type Resposta struct {
	Status   string `json:"status"`
	Mensagem string `json:"mensagem"`
	Dados    any    `json:"dados,omitempty"`
}

/* Cadastro: Dados de criacao de conta; o servidor valida o perfil e os campos antes de armazenar. */
type Cadastro struct {
	Nome   string `json:"nome"`
	Email  string `json:"email"`
	Senha  string `json:"senha"`
	Perfil string `json:"perfil"`
}

/* Credenciais: E-mail e senha enviados para obter uma sessao. */
type Credenciais struct {
	Email string `json:"email"`
	Senha string `json:"senha"`
}

/* Usuario: Dados publicos da conta. Nao inclui senha, salt ou hash. */
type Usuario struct {
	Nome   string `json:"nome"`
	Email  string `json:"email"`
	Perfil string `json:"perfil"`
}

/* Sessao: Token e validade retornados ao cliente depois de autenticar. */
type Sessao struct {
	Token    string  `json:"token"`
	ExpiraEm string  `json:"expira_em"`
	Usuario  Usuario `json:"usuario"`
}

/* OfertaTrecho: Distancia e tempos informados pelo motorista para um par de cidades consecutivas. */
type OfertaTrecho struct {
	DistanciaKM float64 `json:"distancia_km"`
	TempoViagem int     `json:"tempo_viagem_min"`
	TempoParada int     `json:"tempo_parada_min"`
}

/* PublicacaoCarona: Dados de entrada da oferta. Uma rota de N cidades precisa de N-1 trechos. */
type PublicacaoCarona struct {
	ValorKM  float64        `json:"valor_km"`
	Chave    string         `json:"chave"`
	Rota     []string       `json:"rota"`
	DataHora string         `json:"data_hora"`
	Assentos int            `json:"assentos"`
	Trechos  []OfertaTrecho `json:"trechos"`
}

/* Trecho: Aresta do multigrafo. Capacidade e o total; Assentos e a quantidade atualmente livre. */
type Trecho struct {
	Status      string  `json:"status"`
	DistanciaKM float64 `json:"distancia_km"`
	ID          string  `json:"id"`
	CaronaID    string  `json:"carona_id"`
	Origem      string  `json:"origem"`
	Destino     string  `json:"destino"`
	DataHora    string  `json:"data_hora"`
	Assentos    int     `json:"assentos_livres"`
	Capacidade  int     `json:"capacidade"`
	Preco       float64 `json:"preco"`
	TempoViagem int     `json:"tempo_viagem_min"`
	TempoParada int     `json:"tempo_parada_min"`
	Motorista   string  `json:"motorista_email"`
}

/* PassageiroConfirmado: Associacao entre usuario, reserva e numero interno de assento de um trecho. */
type PassageiroConfirmado struct {
	ReservaID  string  `json:"reserva_id"`
	Passageiro Usuario `json:"passageiro"`
	Assento    int     `json:"assento"`
}

/* TrechoConsultado: Trecho junto dos passageiros confirmados, usado na consulta do motorista. */
type TrechoConsultado struct {
	Trecho      Trecho                 `json:"trecho"`
	Passageiros []PassageiroConfirmado `json:"passageiros"`
}

/* Carona: Resposta da oferta com rota, status e detalhes de seus trechos. */
type Carona struct {
	ValorKM   float64            `json:"valor_km"`
	ID        string             `json:"id"`
	Motorista string             `json:"motorista_email"`
	Rota      []string           `json:"rota"`
	DataHora  string             `json:"data_hora"`
	Status    string             `json:"status"`
	Trechos   []TrechoConsultado `json:"trechos"`
}

/* BuscaItinerario: Filtros da busca e criterio de ordenacao; MaxTrechos limita o tamanho do caminho. */
type BuscaItinerario struct {
	Origem     string `json:"origem"`
	Destino    string `json:"destino"`
	Data       string `json:"data"`
	OrdenarPor string `json:"ordenar_por,omitempty"`
	MaxTrechos int    `json:"max_trechos,omitempty"`
}

/* ReservaItinerario: Chave da operacao e IDs em ordem. O cliente nao escolhe um numero de assento. */
type ReservaItinerario struct {
	Chave      string   `json:"chave"`
	TrechosIDs []string `json:"trechos_ids"`
}

/* Identificador: Dados das operacoes que selecionam um recurso pelo ID. */
type Identificador struct {
	ID string `json:"id"`
}

/* Itinerario: Caminho ordenado, preco total e duracao incluindo a espera nas conexoes. */
type Itinerario struct {
	Trechos      []Trecho `json:"trechos"`
	PrecoTotal   float64  `json:"preco_total"`
	DuracaoTotal int      `json:"duracao_total_min"`
}

/* ResultadoBusca: Opcoes encontradas. Limitada avisa que a exploracao pode nao ter coberto todas as alternativas. */
type ResultadoBusca struct {
	Itinerarios []Itinerario `json:"itinerarios"`
	Limitada    bool         `json:"limitada"`
	MaxTrechos  int          `json:"max_trechos"`
}

/* AssentoReservado: Numero atribuido automaticamente e copia dos dados do trecho na reserva. */
type AssentoReservado struct {
	Trecho Trecho `json:"trecho"`
	Numero int    `json:"numero"`
}

/* Reserva: Registro do itinerario confirmado ou cancelado, mantido no historico do passageiro. */
type Reserva struct {
	ID          string             `json:"id"`
	Passageiro  string             `json:"passageiro_email"`
	Status      string             `json:"status"`
	CriadaEm    string             `json:"criada_em"`
	CanceladaEm string             `json:"cancelada_em,omitempty"`
	Motivo      string             `json:"motivo,omitempty"`
	Assentos    []AssentoReservado `json:"assentos"`
	PrecoTotal  float64            `json:"preco_total"`
}

/* Notificacao: Aviso em RAM associado a uma reserva; Lida controla a exibicao dos avisos novos. */
type Notificacao struct {
	ID        string `json:"id"`
	ReservaID string `json:"reserva_id"`
	Mensagem  string `json:"mensagem"`
	CriadaEm  string `json:"criada_em"`
	Lida      bool   `json:"lida"`
}
