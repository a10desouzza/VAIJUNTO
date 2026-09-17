// internal/servidor/gerenciador.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Estado central em RAM: usuarios, sessoes, caronas e reservas.
// Os metodos publicos controlam as travas; os auxiliares dependem da trava de quem os chamou.

package servidor

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
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

/* Guarda os dados comparaveis e o ID gerado para reconhecer uma operacao repetida. */
type repeticao struct {
	assinatura string
	id         string
}

/* GrafoItinerarios: unico estado central. rotas liga cidade de origem aos IDs dos trechos;
 * trechos guarda as arestas por ID; ocupados liga trecho e numero de assento a reserva.
 * agora e uma funcao para os testes controlarem o horario sem esperar a viagem iniciar. */
type GrafoItinerarios struct {
	logger       *log.Logger
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

// NovoGrafo: inicializa os mapas e o relogio. Retorna um servidor sem dados cadastrados. Retorna:
// Ponteiro para GrafoItinerarios com mapas vazios e relogio time.Now.
func NovoGrafo() *GrafoItinerarios {
	return &GrafoItinerarios{
		logger: log.Default(), agora: time.Now, notificacoes: make(map[string][]protocolo.Notificacao),
		usuarios: make(map[string]conta), sessoes: make(map[string]sessao),
		caronas: make(map[string]*oferta), trechos: make(map[string]protocolo.Trecho),
		rotas: make(map[string][]string), ocupados: make(map[string]map[int]string),
		reservas: make(map[string]protocolo.Reserva), chaves: make(map[string]repeticao),
	}
}

// novoID: gera um ID com prefixo e contador. Exige que quem chamou mantenha a trava de escrita.
// Retorna: String com o prefixo e a sequencia incrementada. Exige trava de escrita.
func (g *GrafoItinerarios) novoID(prefixo string) string {
	g.sequencia++
	return fmt.Sprintf("%s%06d", prefixo, g.sequencia)
}

// Cadastrar: valida o cadastro e retorna o usuario ou erro. Armazena salt e hash da senha.
// Retorna: Usuario sem senha quando aceita; estrutura vazia e error quando os dados ou o e-mail
// forem invalidos.
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
	/* Salt aleatorio por conta: senhas iguais nao precisam produzir o mesmo hash armazenado.
	 * Esse calculo ocorre antes da trava de escrita para nao bloquear os mapas durante o PBKDF2. */
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

// Autenticar: confere as credenciais e retorna uma sessao aleatoria com validade de 15 minutos.
// Retorna: Sessao com usuario, identificador e validade; estrutura vazia e error se as credenciais
// forem recusadas.
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

// autorizar: confere sessao, validade e perfil. IMPORTANTE: quem chama ja deve manter Lock ou
// RLock. Retorna: Usuario da sessao ou error se ela expirou, nao existe ou nao permite a operacao.
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

// Desconectar: remove a sessao. Nao apaga a conta nem cancela suas reservas. Retorna: nil ao
// remover a sessao; error quando a sessao nao e valida.
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

func (g *GrafoItinerarios) limparSessoesExpiradas() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.expirarSessoes(g.agora())
}
