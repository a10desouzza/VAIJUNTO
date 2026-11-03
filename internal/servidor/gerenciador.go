// TEC502 - Problema 2. Autor: Arthur de Andrade Souza.
// Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
package servidor

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
	"vaijunto/internal/protocolo"
)

type conta struct {
	usuario protocolo.Usuario
	senha   string
}

type sessao struct {
	email  string
	expira time.Time
}

type oferta struct {
	entrada   protocolo.PublicacaoCarona
	motorista string
	status    string
	ids       []string
}

// Guarda os dados comparaveis e o ID gerado para reconhecer uma operacao repetida.
type repeticao struct {
	assinatura string
	id         string
}

// estadoTrecho agrupa a trava de confirmacao e a ocupacao de uma aresta.
// Escritas tambem usam g.mu, permitindo consultas consistentes sob g.mu.RLock.
type estadoTrecho struct {
	mu         sync.Mutex
	trecho     protocolo.Trecho
	ocupados   map[int]string
	preparados map[int]string
}

// Ordem de travas: manutencao, trechos ordenados, mu. Nunca aguardar um
// trecho mantendo mu. Cancelamentos usam manutencao exclusiva e mu;
// confirmacoes usam manutencao compartilhada e travas dos trechos.
type GrafoItinerarios struct {
	logger       *log.Logger
	agora        func() time.Time
	notificacoes map[string][]protocolo.Notificacao
	mu           sync.RWMutex
	manutencao   sync.RWMutex
	usuarios     map[string]conta
	sessoes      map[string]sessao
	caronas      map[string]*oferta
	trechos      map[string]*estadoTrecho
	rotas        map[string][]string
	reservas     map[string]protocolo.Reserva
	chaves       map[string]repeticao
	sequencia    uint64
	transacoes   map[string]*transacaoLocal
	distribuido  *Distribuido
	backup       *backupLocal
}

// NovoGrafo inicializa os mapas e o relogio. Retorna um servidor sem dados cadastrados.
func NovoGrafo() *GrafoItinerarios {
	return &GrafoItinerarios{
		logger:       log.Default(),
		agora:        time.Now,
		notificacoes: make(map[string][]protocolo.Notificacao),
		usuarios:     make(map[string]conta),
		sessoes:      make(map[string]sessao),
		caronas:      make(map[string]*oferta),
		trechos:      make(map[string]*estadoTrecho),
		rotas:        make(map[string][]string),
		reservas:     make(map[string]protocolo.Reserva),
		chaves:       make(map[string]repeticao),
		transacoes:   make(map[string]*transacaoLocal),
	}
}

// novoID gera um ID com prefixo e contador. Exige que quem chamou mantenha a trava de escrita.
func (g *GrafoItinerarios) novoID(prefixo string) string {
	g.sequencia++
	return fmt.Sprintf("%s%06d", prefixo, g.sequencia)
}

// Cadastrar armazena apenas PBKDF2-SHA256 com salt aleatorio por conta.
func (g *GrafoItinerarios) Cadastrar(c protocolo.Cadastro) (protocolo.Usuario, error) {
	c.Email = strings.ToLower(strings.TrimSpace(c.Email))
	c.Nome = strings.TrimSpace(c.Nome)
	for _, err := range []error{protocolo.ValidarNome(c.Nome), protocolo.ValidarEmail(c.Email), protocolo.ValidarSenha(c.Senha)} {
		if err != nil {
			return protocolo.Usuario{}, err
		}
	}
	if c.Perfil != protocolo.Motorista && c.Perfil != protocolo.Passageiro {
		return protocolo.Usuario{}, fmt.Errorf("perfil deve ser MOTORISTA ou PASSAGEIRO")
	}
	senha, err := protegerSenha(c.Senha)
	if err != nil {
		return protocolo.Usuario{}, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.usuarios[c.Email]; ok {
		return protocolo.Usuario{}, fmt.Errorf("email já cadastrado")
	}
	u := protocolo.Usuario{Nome: c.Nome, Email: c.Email, Perfil: c.Perfil}
	g.usuarios[c.Email] = conta{usuario: u, senha: senha}
	return u, nil
}

// Autenticar confere as credenciais e retorna uma sessao aleatoria com validade de 15 minutos.
func (g *GrafoItinerarios) Autenticar(c protocolo.Credenciais) (protocolo.Sessao, error) {
	email := strings.ToLower(strings.TrimSpace(c.Email))
	if len(c.Senha) > 128 {
		return protocolo.Sessao{}, fmt.Errorf("credenciais inválidas")
	}
	g.mu.RLock()
	usuario, ok := g.usuarios[email]
	g.mu.RUnlock()
	if !ok || !conferirSenha(c.Senha, usuario.senha) {
		return protocolo.Sessao{}, fmt.Errorf("credenciais inválidas")
	}
	aleatorio := make([]byte, 32)
	if _, err := rand.Read(aleatorio); err != nil {
		return protocolo.Sessao{}, err
	}
	sessaoID := hex.EncodeToString(aleatorio)
	agora := g.agora()
	expira := agora.Add(15 * time.Minute)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.expirarSessoes(agora)
	g.sessoes[sessaoID] = sessao{email: email, expira: expira}
	g.logger.Printf("CONECTOU usuário=%s perfil=%s", email, usuario.usuario.Perfil)
	return protocolo.Sessao{ID: sessaoID, ExpiraEm: expira.Format(time.RFC3339), Usuario: usuario.usuario}, nil
}

// autorizar confere sessao, validade e perfil. IMPORTANTE: quem chama ja deve manter Lock ou
// RLock.
func (g *GrafoItinerarios) autorizar(sessaoID, perfil string) (protocolo.Usuario, error) {
	s, ok := g.sessoes[sessaoID]
	if !ok || !s.expira.After(g.agora()) {
		return protocolo.Usuario{}, fmt.Errorf("sessão inválida ou expirada; autentique-se")
	}
	u := g.usuarios[s.email].usuario
	if perfil != "" && u.Perfil != perfil {
		return protocolo.Usuario{}, fmt.Errorf("operação não permitida para este perfil")
	}
	return u, nil
}

// Desconectar remove a sessao. Nao apaga a conta nem cancela suas reservas.
func (g *GrafoItinerarios) Desconectar(sessaoID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, err := g.autorizar(sessaoID, ""); err != nil {
		return err
	}
	g.logger.Printf("DESCONECTOU usuário=%s motivo=saída pelo cliente", g.sessoes[sessaoID].email)
	delete(g.sessoes, sessaoID)
	return nil
}

// expirarSessoes exige trava exclusiva e registra cada expiracao uma unica vez.
func (g *GrafoItinerarios) expirarSessoes(agora time.Time) {
	for id, s := range g.sessoes {
		if !s.expira.After(agora) {
			g.logger.Printf("DESCONECTOU usuário=%s motivo=sessão expirada (15 minutos)", s.email)
			delete(g.sessoes, id)
		}
	}
}

// limparSessoesExpiradas adquire a trava do estado e remove sessoes vencidas usando o horario atual.
func (g *GrafoItinerarios) limparSessoesExpiradas() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.expirarSessoes(g.agora())
}

// Os ponteiros publicados permanecem estaveis; cancelamentos alteram seu status.
func (g *GrafoItinerarios) localizarTrechos(ids []string) (map[string]*estadoTrecho, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	estados := make(map[string]*estadoTrecho, len(ids))
	for _, id := range ids {
		if _, existe := estados[id]; existe {
			return nil, fmt.Errorf("trecho repetido")
		}
		estado, existe := g.trechos[id]
		if !existe {
			return nil, fmt.Errorf("trecho inexistente")
		}
		estados[id] = estado
	}
	return estados, nil
}

// Nunca aguardar travas de trechos mantendo g.mu. IDs ordenados evitam ciclos.
func travarTrechos(estados map[string]*estadoTrecho) func() {
	ids := make([]string, 0, len(estados))
	for id := range estados {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		estados[id].mu.Lock()
	}
	return func() {
		for i := len(ids) - 1; i >= 0; i-- {
			estados[ids[i]].mu.Unlock()
		}
	}
}

// O formato inclui algoritmo, custo, salt e hash. Nunca guarda a senha original.
const custoSenha = 600000

func protegerSenha(senha string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash, err := pbkdf2.Key(sha256.New, senha, salt, custoSenha, 32)
	if err != nil {
		return "", err
	}
	return "pbkdf2-sha256$600000$" + hex.EncodeToString(salt) + "$" + hex.EncodeToString(hash), nil
}
func decodificarSenha(valor string) ([]byte, []byte, error) {
	partes := strings.Split(valor, "$")
	if len(partes) != 4 || partes[0] != "pbkdf2-sha256" || partes[1] != "600000" {
		return nil, nil, fmt.Errorf("hash de senha invalido")
	}
	salt, err := hex.DecodeString(partes[2])
	if err != nil || len(salt) != 16 {
		return nil, nil, fmt.Errorf("salt de senha invalido")
	}
	hash, err := hex.DecodeString(partes[3])
	if err != nil || len(hash) != 32 {
		return nil, nil, fmt.Errorf("hash de senha invalido")
	}
	return salt, hash, nil
}
func conferirSenha(senha, armazenada string) bool {
	salt, esperado, err := decodificarSenha(armazenada)
	if err != nil {
		return false
	}
	hash, err := pbkdf2.Key(sha256.New, senha, salt, custoSenha, 32)
	return err == nil && subtle.ConstantTimeCompare(hash, esperado) == 1
}
