// Arquivo: internal/configuracao/rede.go
// Projeto: Consórcios - Problema 2
// Autor: Arthur de Andrade Souza
// Disciplina: TEC502 - TP01 - Concorrência e Conectividade - 2026.2
// Professor: Antonio A T R Coutinho
// Declaração de não plágio: declaro que não apresento trabalho de terceiros
// como próprio e que as fontes utilizadas devem ser devidamente reconhecidas.
//
// Define os endereços padrão e valida as conexões TCP dos clientes.
// Interpreta identidades e URLs REST usadas na comunicação entre empresas.
// Recusa endereços inválidos, empresas repetidas e configurações incompatíveis.

package configuracao

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// ServidorPadrao atende os testes locais. Para outra maquina, use -servidor
// ou VAIJUNTO_SERVIDOR com o IP e a porta publicados por ela.
const ServidorPadrao = "localhost:8080"

// EscutaPadrao recebe conexoes em todas as interfaces, inclusive no Docker.
// A opcao -endereco :0 pede uma porta livre; o endereco efetivo aparece no log.
const EscutaPadrao = ":8080"

// ServidorCliente associa o nome apresentado no menu ao endereco TCP da empresa.
type ServidorCliente struct {
	Nome     string
	Endereco string
}

// ValidarEnderecoCliente rejeita URLs e mapeamentos Docker antes de pedir credenciais.
// O cliente usa o endereco do computador e apenas a porta TCP publicada nele.
// Recebe: endereco string.
// Retorna: nil se o formato host:porta e o intervalo estiverem validos; erro descritivo caso contrario. Nao testa acesso ao servidor.
func ValidarEnderecoCliente(endereco string) error {
	host, porta, err := net.SplitHostPort(endereco)
	if err != nil || strings.TrimSpace(host) == "" || strings.ContainsAny(host, "/ \t\r\n") {
		return fmt.Errorf("endereco TCP invalido %q: use IP:porta (ex.: 172.16.103.7:8080), sem http:// e sem acrescentar a porta REST ou o mapeamento do Docker", endereco)
	}
	numero, err := strconv.Atoi(porta)
	if err != nil || numero < 1 || numero > 65535 {
		return fmt.Errorf("porta TCP invalida em %q: informe um numero de 1 a 65535", endereco)
	}
	return nil
}

type Empresa struct {
	ID  string
	URL string
}

var identificadorEmpresa = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// ValidarEmpresaID verifica se o identificador da empresa respeita o formato e o tamanho permitidos.
// Recebe: id string.
// Retorna: nil para identidade permitida; erro se o formato ou tamanho forem invalidos.
func ValidarEmpresaID(id string) error {
	if !identificadorEmpresa.MatchString(id) {
		return fmt.Errorf("empresa deve ter de 1 a 64 letras, números, hífens ou sublinhados")
	}
	return nil
}

// Empresas usa o formato empresa-b=http://host:porta,empresa-c=http://host:porta.
// Recebe: texto, propria string.
// Retorna: a lista de pares REST validada ou erro de identidade, duplicacao ou URL.
func Empresas(texto, propria string) ([]Empresa, error) {
	if err := ValidarEmpresaID(propria); err != nil {
		return nil, err
	}
	empresas := make([]Empresa, 0)
	ids := map[string]bool{propria: true}
	if strings.TrimSpace(texto) == "" {
		return empresas, nil
	}
	for _, item := range strings.Split(texto, ",") {
		id, endereco, ok := strings.Cut(strings.TrimSpace(item), "=")
		id, endereco = strings.TrimSpace(id), strings.TrimRight(strings.TrimSpace(endereco), "/")
		if !ok {
			return nil, fmt.Errorf("participante deve usar empresa=http://host:porta")
		}
		if err := ValidarEmpresaID(id); err != nil {
			return nil, err
		}
		if ids[id] {
			return nil, fmt.Errorf("empresa repetida ou própria: %s", id)
		}
		u, err := url.Parse(endereco)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return nil, fmt.Errorf("URL inválida para %s", id)
		}
		ids[id] = true
		empresas = append(empresas, Empresa{ID: id, URL: endereco})
	}
	return empresas, nil
}
