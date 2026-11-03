// Autor: Arthur de Andrade Souza.
// Matéria: TEC502 - TP01 - Concorrência e Conectividade - 2026.2.
// Declaração de não plágio: Fontes utilizadas devem ser reconhecidas; nao apresentar trabalho de terceiros como proprio.
// Funcionalidade: Define e valida os endereços TCP dos clientes e as identidades e URLs REST das empresas.
package configuracao

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const ServidorPadrao = "localhost:8080"

const EscutaPadrao = ":8080"

type ServidorCliente struct {
	Nome     string
	Endereco string
}

// ValidarEnderecoCliente rejeita URLs e mapeamentos Docker antes de pedir credenciais. O cliente usa o endereco do computador e apenas a porta TCP publicada nele.
// Recebe: endereço TCP no formato host:porta.
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
// Recebe: identificador da empresa.
func ValidarEmpresaID(id string) error {
	if !identificadorEmpresa.MatchString(id) {
		return fmt.Errorf("empresa deve ter de 1 a 64 letras, números, hífens ou sublinhados")
	}
	return nil
}

// Empresas usa o formato empresa-b=http://host:porta,empresa-c=http://host:porta.
// Recebe: lista textual dos pares e ID da empresa local.
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
