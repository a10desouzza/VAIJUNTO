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

type Requisicao struct {
	Acao  string          `json:"acao"`
	Token string          `json:"token,omitempty"`
	Dados json.RawMessage `json:"dados"`
}

type Resposta struct {
	Status   string `json:"status"`
	Mensagem string `json:"mensagem"`
	Dados    any    `json:"dados,omitempty"`
}

type Cadastro struct {
	Nome   string `json:"nome"`
	Email  string `json:"email"`
	Senha  string `json:"senha"`
	Perfil string `json:"perfil"`
}

type Credenciais struct {
	Email string `json:"email"`
	Senha string `json:"senha"`
}

type Usuario struct {
	Nome   string `json:"nome"`
	Email  string `json:"email"`
	Perfil string `json:"perfil"`
}

type Sessao struct {
	Token    string  `json:"token"`
	ExpiraEm string  `json:"expira_em"`
	Usuario  Usuario `json:"usuario"`
}

type OfertaTrecho struct {
	DistanciaKM float64 `json:"distancia_km"`
	TempoViagem int     `json:"tempo_viagem_min"`
	TempoParada int     `json:"tempo_parada_min"`
}

type PublicacaoCarona struct {
	ValorKM  float64        `json:"valor_km"`
	Chave    string         `json:"chave"`
	Rota     []string       `json:"rota"`
	DataHora string         `json:"data_hora"`
	Assentos int            `json:"assentos"`
	Trechos  []OfertaTrecho `json:"trechos"`
}

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

type PassageiroConfirmado struct {
	ReservaID  string  `json:"reserva_id"`
	Passageiro Usuario `json:"passageiro"`
	Assento    int     `json:"assento"`
}

type TrechoConsultado struct {
	Trecho      Trecho                 `json:"trecho"`
	Passageiros []PassageiroConfirmado `json:"passageiros"`
}

type Carona struct {
	ValorKM   float64            `json:"valor_km"`
	ID        string             `json:"id"`
	Motorista string             `json:"motorista_email"`
	Rota      []string           `json:"rota"`
	DataHora  string             `json:"data_hora"`
	Status    string             `json:"status"`
	Trechos   []TrechoConsultado `json:"trechos"`
}

type BuscaItinerario struct {
	Origem     string `json:"origem"`
	Destino    string `json:"destino"`
	Data       string `json:"data"`
	OrdenarPor string `json:"ordenar_por,omitempty"`
	MaxTrechos int    `json:"max_trechos,omitempty"`
}

type ReservaItinerario struct {
	Chave      string   `json:"chave"`
	TrechosIDs []string `json:"trechos_ids"`
}

type Identificador struct {
	ID string `json:"id"`
}

type Itinerario struct {
	Trechos      []Trecho `json:"trechos"`
	PrecoTotal   float64  `json:"preco_total"`
	DuracaoTotal int      `json:"duracao_total_min"`
}

type ResultadoBusca struct {
	Itinerarios []Itinerario `json:"itinerarios"`
	Limitada    bool         `json:"limitada"`
	MaxTrechos  int          `json:"max_trechos"`
}

type AssentoReservado struct {
	Trecho Trecho `json:"trecho"`
	Numero int    `json:"numero"`
}

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

type Notificacao struct {
	ID        string `json:"id"`
	ReservaID string `json:"reserva_id"`
	Mensagem  string `json:"mensagem"`
	CriadaEm  string `json:"criada_em"`
	Lida      bool   `json:"lida"`
}
