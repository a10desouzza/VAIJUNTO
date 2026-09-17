// internal/protocolo/validacao.go - VaiJunto: sistema de caronas compartilhadas
// Autor: Arthur Souza
// Validacoes compartilhadas pela interface e pelo servidor.
// Validar no cliente ajuda na digitacao, mas nao substitui a validacao no servidor.

package protocolo

import (
	"fmt"
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ValidarEmail: confere o formato e o dominio. Nao verifica se a caixa de e-mail realmente existe.
// Retorna: nil se o formato for aceito; error com a regra violada. Nao confirma a existencia da
// caixa de e-mail.
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

// ValidarNome: confere tamanho, letras e separadores aceitos no nome. Retorna: nil para nome
// aceito; error se tamanho, caracteres ou quantidade de letras forem invalidos.
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

// ValidarSenha: confere tamanho e caracteres de controle. Retorna nil quando o formato e aceito.
// Retorna: nil se atender ao tamanho e caracteres permitidos; error caso contrario.
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

// ValidarCidade: aceita acentos e numeros, mas exige ao menos uma letra e limita o tamanho.
// Retorna: nil quando preenchida e com caracteres aceitos; error se violar as regras.
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
