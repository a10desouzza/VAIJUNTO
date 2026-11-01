// Arquivo: internal/servidor/backup.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Grava o estado privado da empresa para recuperá-lo após a queda do processo.
// Inclui dados locais, preparações e registros necessários para preservar o consenso.
// Valida o conteúdo na restauração e sincroniza um temporário antes de substituir o arquivo.
// Cada empresa usa seu próprio arquivo; o backup não é um armazenamento compartilhado.

package servidor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"vaijunto/internal/protocolo"
)

// O arquivo pertence a uma empresa. Ele guarda o estado local e as promessas
// dessa testemunha, sem copiar cadastros ou ofertas das outras empresas.
type backupLocal struct {
	mu      sync.Mutex
	caminho string
	empresa string
}

type arquivoBackup struct {
	Versao   int             `json:"versao"`
	Empresa  string          `json:"empresa"`
	SalvoEm  time.Time       `json:"salvo_em"`
	Checksum string          `json:"sha256"`
	Estado   json.RawMessage `json:"estado"`
}

type estadoBackup struct {
	Usuarios     map[string]contaBackup             `json:"usuarios"`
	Caronas      map[string]ofertaBackup            `json:"caronas"`
	Trechos      map[string]trechoBackup            `json:"trechos"`
	Reservas     map[string]protocolo.Reserva       `json:"reservas"`
	Chaves       map[string]chaveBackup             `json:"chaves"`
	Notificacoes map[string][]protocolo.Notificacao `json:"notificacoes"`
	Transacoes   map[string]transacaoBackup         `json:"transacoes"`
	Sequencia    uint64                             `json:"sequencia"`
	Consorcio    *consorcioBackup                   `json:"consorcio,omitempty"`
}

type contaBackup struct {
	Usuario protocolo.Usuario `json:"usuario"`
	Senha   string            `json:"senha"`
}

type ofertaBackup struct {
	Entrada   protocolo.PublicacaoCarona `json:"entrada"`
	Motorista string                     `json:"motorista"`
	Status    string                     `json:"status"`
	IDs       []string                   `json:"trechos"`
}

type trechoBackup struct {
	Trecho     protocolo.Trecho `json:"trecho"`
	Ocupados   map[int]string   `json:"ocupados"`
	Preparados map[int]string   `json:"preparados"`
}

type chaveBackup struct {
	Assinatura string `json:"assinatura"`
	ID         string `json:"id"`
}

type transacaoBackup struct {
	Pedido    protocolo.PedidoPreparacao `json:"pedido"`
	Estado    protocolo.EstadoTransacao  `json:"estado"`
	Assentos  map[string]int             `json:"assentos"`
	Motivo    string                     `json:"motivo"`
	ReservaID string                     `json:"reserva_id"`
	Ordem     uint64                     `json:"ordem"`
}

type consorcioBackup struct {
	Geracao   string                       `json:"geracao"`
	Relogio   uint64                       `json:"relogio"`
	Membros   []string                     `json:"membros"`
	TokenHash string                       `json:"token_hash"`
	Registros map[string]*registroConsenso `json:"registros"`
	Origens   map[string]*operacaoOrigem   `json:"origens"`
}

// AbrirBackup deve ser chamado antes de abrir os atendimentos e a recuperacao.
// Caminho vazio desativa a persistencia. Uma copia invalida nunca e descartada
// silenciosamente reiniciar sem as promessas antigas permitiria votos conflitantes.
// Recebe: empresa, caminho string.
// Retorna: nil se o arquivo foi restaurado/criado ou a persistencia estiver desativada; erro se leitura, validacao ou gravacao falhar.
func (g *GrafoItinerarios) AbrirBackup(empresa, caminho string) error {
	if caminho == "" {
		return nil
	}
	abs, err := filepath.Abs(caminho)
	if err != nil {
		return err
	}
	b := &backupLocal{caminho: abs, empresa: empresa}
	conteudo, err := os.ReadFile(abs)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("ler backup: %w", err)
	}
	if err == nil {
		var arquivo arquivoBackup
		if err := json.Unmarshal(conteudo, &arquivo); err != nil {
			return fmt.Errorf("backup invalido: %w", err)
		}
		if arquivo.Versao != 1 || arquivo.Empresa != empresa || hashBytes(arquivo.Estado) != arquivo.Checksum {
			return fmt.Errorf("backup com versao, empresa ou checksum divergente")
		}
		var estado estadoBackup
		if err := json.Unmarshal(arquivo.Estado, &estado); err != nil {
			return fmt.Errorf("estado do backup invalido: %w", err)
		}
		if err := g.restaurarBackup(estado); err != nil {
			return err
		}
		g.logger.Printf("BACKUP recuperado empresa=%s arquivo=%s", empresa, abs)
	}
	g.backup = b
	return g.SalvarBackup()
}

// SalvarBackup grava e sincroniza um temporario antes de substituir o arquivo.
// O sucesso de uma escrita so e respondido depois dessa etapa. A trava do backup
// impede que uma gravacao antiga sobrescreva uma fotografia mais recente.
// Recebe: nenhum parametro.
// Retorna: nil depois do checkpoint ou se desativado; erro de captura ou gravacao impede confirmar sucesso ao cliente.
func (g *GrafoItinerarios) SalvarBackup() error {
	b := g.backup
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	estado, err := g.fotografarEstado()
	if err != nil {
		return err
	}
	conteudo, err := json.Marshal(arquivoBackup{Versao: 1, Empresa: b.empresa, SalvoEm: time.Now().UTC(), Checksum: hashBytes(estado), Estado: estado})
	if err != nil {
		return err
	}
	if err := substituirArquivo(b.caminho, conteudo); err != nil {
		return fmt.Errorf("backup nao gravado; resultado da operacao pode estar pendente: %w", err)
	}
	return nil
}

// hashBytes calcula o checksum SHA-256 dos bytes do estado salvo.
// Recebe: conteudo []byte.
// Retorna: string.
func hashBytes(conteudo []byte) string {
	hash := sha256.Sum256(conteudo)
	return hex.EncodeToString(hash[:])
}

// fotografarEstado captura dados locais e votos sob as travas de leitura do estado.
// Recebe: nenhum parametro.
// Retorna: os bytes JSON de uma fotografia consistente ou erro de serializacao.
func (g *GrafoItinerarios) fotografarEstado() ([]byte, error) {
	d := g.distribuido
	if d != nil {
		d.mu.Lock()
		defer d.mu.Unlock()
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	e := estadoBackup{
		Usuarios:   make(map[string]contaBackup, len(g.usuarios)),
		Caronas:    make(map[string]ofertaBackup, len(g.caronas)),
		Trechos:    make(map[string]trechoBackup, len(g.trechos)),
		Chaves:     make(map[string]chaveBackup, len(g.chaves)),
		Transacoes: make(map[string]transacaoBackup, len(g.transacoes)),
		Reservas:   g.reservas, Notificacoes: g.notificacoes, Sequencia: g.sequencia,
	}
	for email, c := range g.usuarios {
		e.Usuarios[email] = contaBackup{c.usuario, c.senha}
	}
	for id, o := range g.caronas {
		e.Caronas[id] = ofertaBackup{o.entrada, o.motorista, o.status, o.ids}
	}
	for id, t := range g.trechos {
		e.Trechos[id] = trechoBackup{t.trecho, t.ocupados, t.preparados}
	}
	for id, c := range g.chaves {
		e.Chaves[id] = chaveBackup{c.assinatura, c.id}
	}
	for id, t := range g.transacoes {
		e.Transacoes[id] = transacaoBackup{t.pedido, t.estado, t.assentos, t.motivo, t.reservaID, t.ordem}
	}
	if d != nil {
		e.Consorcio = &consorcioBackup{d.geracao, d.relogio, d.membros, hashBytes([]byte(d.token)), d.registros, d.origens}
	}
	return json.Marshal(e)
}

// restaurarBackup valida as referencias antes de remontar os mapas e o protocolo.
// Recebe: e estadoBackup.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
func (g *GrafoItinerarios) restaurarBackup(e estadoBackup) error {
	if e.Usuarios == nil || e.Caronas == nil || e.Trechos == nil || e.Reservas == nil || e.Chaves == nil || e.Transacoes == nil || e.Notificacoes == nil {
		return fmt.Errorf("backup incompleto")
	}
	if c := e.Consorcio; c != nil {
		d := g.distribuido
		if d == nil || c.Geracao == "" || c.Registros == nil || c.Origens == nil || hashJSON(c.Membros) != hashJSON(d.membros) || c.TokenHash != hashBytes([]byte(d.token)) {
			return fmt.Errorf("backup exige a mesma empresa, membros e token do consorcio")
		}
	}
	for id, t := range e.Trechos {
		if t.Trecho.ID != id || t.Ocupados == nil || t.Preparados == nil || t.Trecho.Assentos != t.Trecho.Capacidade-len(t.Ocupados)-len(t.Preparados) {
			return fmt.Errorf("ocupacao inconsistente no backup do trecho %s", id)
		}
		if _, existe := e.Caronas[t.Trecho.CaronaID]; !existe {
			return fmt.Errorf("carona ausente no backup do trecho %s", id)
		}
		for n, reserva := range t.Ocupados {
			if _, existe := e.Reservas[reserva]; !existe || n < 1 || n > t.Trecho.Capacidade || t.Preparados[n] != "" {
				return fmt.Errorf("assento ocupado inconsistente em %s", id)
			}
		}
		for n, tx := range t.Preparados {
			p, existe := e.Transacoes[tx]
			if !existe || p.Estado != protocolo.TransacaoPreparada || p.Assentos[id] != n || n < 1 || n > t.Trecho.Capacidade {
				return fmt.Errorf("preparacao inconsistente em %s", id)
			}
		}
	}
	for _, o := range e.Caronas {
		for _, id := range o.IDs {
			if _, existe := e.Trechos[id]; !existe {
				return fmt.Errorf("trecho %s ausente no backup da carona", id)
			}
		}
	}
	for email, c := range e.Usuarios {
		g.usuarios[email] = conta{c.Usuario, c.Senha}
	}
	for id, o := range e.Caronas {
		g.caronas[id] = &oferta{o.Entrada, o.Motorista, o.Status, o.IDs}
	}
	for id, t := range e.Trechos {
		g.trechos[id] = &estadoTrecho{trecho: t.Trecho, ocupados: t.Ocupados, preparados: t.Preparados}
		origem := chaveCidade(t.Trecho.Origem)
		g.rotas[origem] = append(g.rotas[origem], id)
	}
	for id, c := range e.Chaves {
		g.chaves[id] = repeticao{c.Assinatura, c.ID}
	}
	for id, t := range e.Transacoes {
		g.transacoes[id] = &transacaoLocal{t.Pedido, t.Estado, t.Assentos, t.Motivo, t.ReservaID, t.Ordem}
	}
	g.reservas, g.notificacoes, g.sequencia = e.Reservas, e.Notificacoes, e.Sequencia
	if c := e.Consorcio; c != nil {
		d := g.distribuido
		d.geracao, d.relogio, d.registros, d.origens = c.Geracao, c.Relogio, c.Registros, c.Origens
	}
	// Sessoes sao temporarias: o usuario entra novamente apos a recuperacao.
	return nil
}

// substituirArquivo conserva o arquivo atual ate a nova gravacao estar sincronizada.
// Recebe: caminho string, conteudo []byte.
// Retorna: error: nil quando a operacao termina sem erro; caso contrario, o motivo da falha.
func substituirArquivo(caminho string, conteudo []byte) error {
	pasta := filepath.Dir(caminho)
	if err := os.MkdirAll(pasta, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(pasta, ".estado-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(conteudo); err == nil {
		err = f.Sync()
	}
	if fecharErr := f.Close(); err == nil {
		err = fecharErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(f.Name(), caminho); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		dir, err := os.Open(pasta)
		if err != nil {
			return err
		}
		defer dir.Close()
		return dir.Sync()
	}
	return nil
}
