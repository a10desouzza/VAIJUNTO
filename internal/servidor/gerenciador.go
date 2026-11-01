// Arquivo: internal/servidor/gerenciador.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Mantém em memória contas, sessões, ofertas, trechos e reservas da empresa.
// Implementa autenticação, geração de identificadores e acesso aos estados dos trechos.
// Define as travas e sua ordem de aquisição para coordenar operações locais concorrentes.

package servidor

import (
	"crypto/rand"
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
// Recebe: nenhum parametro.
// Retorna: um ponteiro para o estado local inicializado com mapas vazios e relogio de horario real.
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
// Recebe: prefixo string.
// Retorna: string.
func (g *GrafoItinerarios) novoID(prefixo string) string {
	g.sequencia++
	return fmt.Sprintf("%s%06d", prefixo, g.sequencia)
}

// Cadastrar guarda a senha em texto puro na memoria para o prototipo didatico.
// Recebe: c protocolo.Cadastro.
// Retorna: (protocolo.Usuario, error). Em falha, devolve error; em sucesso, erro nil.
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
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.usuarios[c.Email]; ok {
		return protocolo.Usuario{}, fmt.Errorf("email já cadastrado")
	}
	u := protocolo.Usuario{Nome: c.Nome, Email: c.Email, Perfil: c.Perfil}
	g.usuarios[c.Email] = conta{usuario: u, senha: c.Senha}
	return u, nil
}

// Autenticar confere as credenciais e retorna uma sessao aleatoria com validade de 15 minutos.
// Recebe: c protocolo.Credenciais.
// Retorna: (protocolo.Sessao, error). Em falha, devolve error; em sucesso, erro nil.
func (g *GrafoItinerarios) Autenticar(c protocolo.Credenciais) (protocolo.Sessao, error) {
	email := strings.ToLower(strings.TrimSpace(c.Email))
	if len(c.Senha) > 128 {
		return protocolo.Sessao{}, fmt.Errorf("credenciais inválidas")
	}
	g.mu.RLock()
	usuario, ok := g.usuarios[email]
	g.mu.RUnlock()
	if !ok || c.Senha != usuario.senha {
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
// Recebe: sessaoID, perfil string.
// Retorna: (protocolo.Usuario, error). Em falha, devolve error; em sucesso, erro nil.
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
// Recebe: sessaoID string.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
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
// Recebe: agora time.Time.
// Retorna: nenhum valor; executa os efeitos descritos acima.
func (g *GrafoItinerarios) expirarSessoes(agora time.Time) {
	for id, s := range g.sessoes {
		if !s.expira.After(agora) {
			g.logger.Printf("DESCONECTOU usuário=%s motivo=sessão expirada (15 minutos)", s.email)
			delete(g.sessoes, id)
		}
	}
}

// limparSessoesExpiradas adquire a trava do estado e remove sessoes vencidas usando o horario atual.
// Recebe: nenhum parametro.
// Retorna: nenhum valor; executa os efeitos descritos acima.
func (g *GrafoItinerarios) limparSessoesExpiradas() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.expirarSessoes(g.agora())
}

// Os ponteiros publicados permanecem estaveis; cancelamentos alteram seu status.
// Recebe: ids []string.
// Retorna: o mapa de IDs para estados locais ou erro de trecho repetido/inexistente; os ponteiros continuam sujeitos as travas do grafo.
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
// Recebe: estados map[string]*estadoTrecho.
// Retorna: uma funcao que libera as travas adquiridas; o chamador deve executa-la ao terminar a operacao protegida.
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
