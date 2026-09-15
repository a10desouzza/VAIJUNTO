package servidor

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
	"vaijunto/internal/protocolo"
)

type conta struct {
	usuario protocolo.Usuario
	salt    []byte
	hash    []byte
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

type repeticao struct {
	assinatura string
	id         string
}

type GrafoItinerarios struct {
	agora        func() time.Time
	notificacoes map[string][]protocolo.Notificacao
	mu           sync.RWMutex
	usuarios     map[string]conta
	sessoes      map[string]sessao
	caronas      map[string]*oferta
	trechos      map[string]protocolo.Trecho
	rotas        map[string][]string
	ocupados     map[string]map[int]string
	reservas     map[string]protocolo.Reserva
	chaves       map[string]repeticao
	sequencia    uint64
}

func NovoGrafo() *GrafoItinerarios {
	return &GrafoItinerarios{
		agora: time.Now, notificacoes: make(map[string][]protocolo.Notificacao),
		usuarios: make(map[string]conta), sessoes: make(map[string]sessao),
		caronas: make(map[string]*oferta), trechos: make(map[string]protocolo.Trecho),
		rotas: make(map[string][]string), ocupados: make(map[string]map[int]string),
		reservas: make(map[string]protocolo.Reserva), chaves: make(map[string]repeticao),
	}
}

func (g *GrafoItinerarios) novoID(prefixo string) string {
	g.sequencia++
	return fmt.Sprintf("%s%06d", prefixo, g.sequencia)
}

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
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return protocolo.Usuario{}, err
	}
	hash, err := pbkdf2.Key(sha256.New, c.Senha, salt, 60000, 32)
	if err != nil {
		return protocolo.Usuario{}, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.usuarios[c.Email]; ok {
		return protocolo.Usuario{}, fmt.Errorf("email já cadastrado")
	}
	u := protocolo.Usuario{Nome: c.Nome, Email: c.Email, Perfil: c.Perfil}
	g.usuarios[c.Email] = conta{usuario: u, salt: salt, hash: hash}
	return u, nil
}

func (g *GrafoItinerarios) Autenticar(c protocolo.Credenciais) (protocolo.Sessao, error) {
	email := strings.ToLower(strings.TrimSpace(c.Email))
	if len(c.Senha) > 128 {
		return protocolo.Sessao{}, fmt.Errorf("credenciais inválidas")
	}
	g.mu.RLock()
	usuario, ok := g.usuarios[email]
	g.mu.RUnlock()
	if !ok {
		return protocolo.Sessao{}, fmt.Errorf("credenciais inválidas")
	}
	hash, err := pbkdf2.Key(sha256.New, c.Senha, usuario.salt, 60000, 32)
	if err != nil || subtle.ConstantTimeCompare(hash, usuario.hash) != 1 {
		return protocolo.Sessao{}, fmt.Errorf("credenciais inválidas")
	}
	aleatorio := make([]byte, 32)
	if _, err := rand.Read(aleatorio); err != nil {
		return protocolo.Sessao{}, err
	}
	token := hex.EncodeToString(aleatorio)
	agora := g.agora()
	expira := agora.Add(2 * time.Hour)
	g.mu.Lock()
	defer g.mu.Unlock()
	for chave, s := range g.sessoes {
		if !s.expira.After(agora) {
			delete(g.sessoes, chave)
		}
	}
	g.sessoes[token] = sessao{email: email, expira: expira}
	return protocolo.Sessao{Token: token, ExpiraEm: expira.Format(time.RFC3339), Usuario: usuario.usuario}, nil
}

func (g *GrafoItinerarios) autorizar(token, perfil string) (protocolo.Usuario, error) {
	s, ok := g.sessoes[token]
	if !ok || !s.expira.After(g.agora()) {
		return protocolo.Usuario{}, fmt.Errorf("sessão inválida ou expirada; autentique-se")
	}
	u := g.usuarios[s.email].usuario
	if perfil != "" && u.Perfil != perfil {
		return protocolo.Usuario{}, fmt.Errorf("operação não permitida para este perfil")
	}
	return u, nil
}

func (g *GrafoItinerarios) Desconectar(token string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, err := g.autorizar(token, ""); err != nil {
		return err
	}
	delete(g.sessoes, token)
	return nil
}
