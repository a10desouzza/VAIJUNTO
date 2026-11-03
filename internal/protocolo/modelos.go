// Autor: Arthur de Andrade Souza.
// Matéria: TEC502 - TP01 - Concorrência e Conectividade - 2026.2.
// Declaração de não plágio: Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
// Funcionalidade: Define os contratos JSON de usuários, viagens, reservas e transações e valida os dados de entrada.
package protocolo

import (
	"encoding/json"
	"fmt"
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"
)

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
	Acao     string          `json:"acao"`
	SessaoID string          `json:"sessao,omitempty"`
	Dados    json.RawMessage `json:"dados"`
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
	ID       string  `json:"id"`
	ExpiraEm string  `json:"expira_em"`
	Usuario  Usuario `json:"usuario"`
}

type OfertaTrecho struct {
	Preco       float64 `json:"preco"`
	DistanciaKM float64 `json:"distancia_km"`
	TempoViagem int     `json:"tempo_viagem_min"`
	TempoParada int     `json:"tempo_parada_min"`
}

type PublicacaoCarona struct {
	Chave    string         `json:"chave"`
	Rota     []string       `json:"rota"`
	DataHora string         `json:"data_hora"`
	Assentos int            `json:"assentos"`
	Trechos  []OfertaTrecho `json:"trechos"`
}

type Trecho struct {
	EmpresaID   string  `json:"empresa_id,omitempty"`
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
	TransacaoID   string  `json:"transacao_id,omitempty"`
	EmpresaOrigem string  `json:"empresa_origem,omitempty"`
	PassageiroRef string  `json:"passageiro_ref,omitempty"`
	ReservaID     string  `json:"reserva_id"`
	Passageiro    Usuario `json:"passageiro"`
	Assento       int     `json:"assento"`
}

type TrechoConsultado struct {
	Trecho      Trecho                 `json:"trecho"`
	Passageiros []PassageiroConfirmado `json:"passageiros"`
}

type Carona struct {
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
	MaxTrechos int    `json:"max_trechos,omitempty"`
}

type ReservaItinerario struct {
	Trechos    []ReferenciaTrecho `json:"trechos,omitempty"`
	Chave      string             `json:"chave"`
	TrechosIDs []string           `json:"trechos_ids"`
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
	Indisponiveis []string     `json:"empresas_indisponiveis,omitempty"`
	Itinerarios   []Itinerario `json:"itinerarios"`
	Limitada      bool         `json:"limitada"`
	MaxTrechos    int          `json:"max_trechos"`
}

type AssentoReservado struct {
	Trecho Trecho `json:"trecho"`
	Numero int    `json:"numero"`
}

type Reserva struct {
	EmpresasIndisponiveis []string           `json:"empresas_indisponiveis,omitempty"`
	InicioItinerario      string             `json:"inicio_itinerario,omitempty"`
	TransacaoID           string             `json:"transacao_id,omitempty"`
	EmpresaOrigem         string             `json:"empresa_origem,omitempty"`
	PassageiroRef         string             `json:"passageiro_ref,omitempty"`
	ID                    string             `json:"id"`
	Passageiro            string             `json:"passageiro_email"`
	Status                string             `json:"status"`
	CriadaEm              string             `json:"criada_em"`
	CanceladaEm           string             `json:"cancelada_em,omitempty"`
	Motivo                string             `json:"motivo,omitempty"`
	Assentos              []AssentoReservado `json:"assentos"`
	PrecoTotal            float64            `json:"preco_total"`
}

type Notificacao struct {
	ID        string `json:"id"`
	ReservaID string `json:"reserva_id"`
	Mensagem  string `json:"mensagem"`
	CriadaEm  string `json:"criada_em"`
	Lida      bool   `json:"lida"`
}

type ReferenciaTrecho struct {
	EmpresaID string `json:"empresa_id"`
	TrechoID  string `json:"trecho_id"`
}

type ReservaDistribuida struct {
	Chave   string             `json:"chave"`
	Trechos []ReferenciaTrecho `json:"trechos"`
}

type EstadoTransacao string

const (
	TransacaoPreparada  EstadoTransacao = "PREPARADA"
	TransacaoConfirmada EstadoTransacao = "CONFIRMADA"
	TransacaoAbortada   EstadoTransacao = "ABORTADA"
)

type PedidoPreparacao struct {
	InicioItinerario string   `json:"inicio_itinerario,omitempty"`
	TransacaoID      string   `json:"transacao_id"`
	EmpresaOrigem    string   `json:"empresa_origem"`
	PassageiroRef    string   `json:"passageiro_ref"`
	Participantes    []string `json:"participantes"`
	TrechosIDs       []string `json:"trechos_ids"`
}

type RespostaPreparacao struct {
	TransacaoID string          `json:"transacao_id"`
	EmpresaID   string          `json:"empresa_id"`
	Estado      EstadoTransacao `json:"estado"`
	Motivo      string          `json:"motivo,omitempty"`
	ReservaID   string          `json:"reserva_id,omitempty"`
}

// ValidarEmail valida tamanho, codificação e formato do e-mail, incluindo o domínio.
// Recebe: e-mail digitado.
func ValidarEmail(email string) error {
	if len(email) > 254 || !utf8.ValidString(email) {
		return fmt.Errorf("e-mail deve ter até 254 bytes")
	}
	endereco, err := mail.ParseAddress(email)
	if err != nil || endereco.Address != email {
		return fmt.Errorf("informe um e-mail válido, como nome@exemplo.com")
	}
	partes := strings.Split(email, "@")
	if len(partes) != 2 || len(partes[0]) > 64 || strings.ContainsAny(partes[0], "\" ") {
		return fmt.Errorf("informe um e-mail válido, como nome@exemplo.com")
	}
	dominio := strings.Split(partes[1], ".")
	if len(dominio) < 2 {
		return fmt.Errorf("o domínio do e-mail precisa de uma extensão, como .com ou .com.br")
	}
	for _, parte := range dominio {
		if len(parte) == 0 || len(parte) > 63 || parte[0] == '-' || parte[len(parte)-1] == '-' {
			return fmt.Errorf("domínio de e-mail inválido")
		}
		for _, r := range parte {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return fmt.Errorf("domínio de e-mail inválido")
			}
		}
	}
	return nil
}

// ValidarNome valida tamanho e caracteres do nome, exigindo ao menos duas letras.
// Recebe: nome digitado.
func ValidarNome(nome string) error {
	if !utf8.ValidString(nome) || len(nome) > 100 || utf8.RuneCountInString(nome) < 2 {
		return fmt.Errorf("nome deve ter ao menos 2 caracteres e até 100 bytes")
	}
	letras := 0
	for _, r := range nome {
		if unicode.IsLetter(r) {
			letras++
			continue
		}
		if !unicode.IsMark(r) && r != ' ' && r != '\'' && r != '’' && r != '-' && r != '.' {
			return fmt.Errorf("use letras, espaços, hífen ou apóstrofo no nome")
		}
	}
	if letras < 2 {
		return fmt.Errorf("nome deve conter ao menos duas letras")
	}
	return nil
}

// ValidarSenha valida a entrada da senha, sem executar sua derivação criptográfica.
// Recebe: senha digitada.
func ValidarSenha(senha string) error {
	if !utf8.ValidString(senha) || utf8.RuneCountInString(senha) < 8 || len(senha) > 128 || strings.TrimSpace(senha) == "" {
		return fmt.Errorf("senha deve ter ao menos 8 caracteres e até 128 bytes")
	}
	for _, r := range senha {
		if unicode.IsControl(r) {
			return fmt.Errorf("senha não pode conter caracteres de controle")
		}
	}
	return nil
}

// ValidarCidade valida preenchimento, tamanho e caracteres do nome da cidade.
// Recebe: nome da cidade.
func ValidarCidade(cidade string) error {
	if !utf8.ValidString(cidade) || len(cidade) > 100 || strings.TrimSpace(cidade) == "" {
		return fmt.Errorf("cidade deve ser preenchida e ter até 100 bytes")
	}
	letras := 0
	for _, r := range cidade {
		if unicode.IsLetter(r) {
			letras++
			continue
		}
		if !unicode.IsMark(r) && !unicode.IsDigit(r) && r != ' ' && r != '-' && r != '\'' && r != '’' && r != '.' {
			return fmt.Errorf("cidade deve conter letras; números, espaços, hífen e apóstrofo também são aceitos")
		}
	}
	if letras == 0 {
		return fmt.Errorf("cidade deve conter letras")
	}
	return nil
}
