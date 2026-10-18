// Package normalize normaliza o texto lido por tipo de campo, devolvendo o
// valor normalizado e a lista de transformações aplicadas (proveniência).
package normalize

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"

	"github.com/Fontoura21/ocr2act-go/internal/layout"
)

// Nomes das transformações registradas na proveniência de cada campo.
const (
	Uppercase          = "maiusculas"
	RemoveAccents      = "remove_acentos"
	RemoveSymbols      = "remove_simbolos_e_espacos"
	RemovePunctuation  = "remove_pontuacao_e_espacos"
	UnreadableDate     = "data_ilegivel"
	InvalidDate        = "data_invalida"
	DateToISO          = "ddmmaaaa_para_iso8601"
	CanonicalUppercase = "maiusculas_sem_acento_espacos"
)

// Upper converte para maiúsculas com o mapeamento completo do Unicode
// (ß vira SS), como o str.upper do Python.
func Upper(s string) string {
	return cases.Upper(language.Und).String(s)
}

// StripAccents decompõe o texto (NFKD) e descarta as marcas combinantes.
func StripAccents(s string) string {
	d := norm.NFKD.String(s)
	var b strings.Builder
	b.Grow(len(d))
	for i, r := range d {
		if norm.NFKD.PropertiesString(d[i:]).CCC() == 0 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// IsSpace é o critério de espaço em branco do Python (str.isspace), que
// inclui os separadores de 0x1C a 0x1F além do que unicode.IsSpace aceita.
func IsSpace(r rune) bool {
	return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f)
}

// CollapseSpaces troca cada sequência de espaços por um único espaço e apara
// as pontas.
func CollapseSpaces(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	pending := false
	for _, r := range s {
		if IsSpace(r) {
			pending = b.Len() > 0
			continue
		}
		if pending {
			b.WriteByte(' ')
			pending = false
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Name normaliza nomes de pessoas: maiúsculas, sem acento e só com letras A-Z
// separadas por um espaço.
func Name(s string) (string, []string) {
	t := []string{}
	out := Upper(s)
	if out != s {
		t = append(t, Uppercase)
	}
	if plain := StripAccents(out); plain != out {
		out = plain
		t = append(t, RemoveAccents)
	}
	cleaned := CollapseSpaces(strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || r == ' ' {
			return r
		}
		return ' '
	}, out))
	if cleaned != out {
		t = append(t, RemoveSymbols)
	}
	return cleaned, t
}

// Identifier remove pontuação e espaços de CPF e matrícula. Mantém letras de
// propósito: um "B" no lugar de "8" é evidência de erro de OCR.
func Identifier(s string) (string, []string) {
	out := KeepAlphanumeric(s)
	if out != s {
		return out, []string{RemovePunctuation}
	}
	return out, []string{}
}

// KeepAlphanumeric mantém só [0-9A-Za-z|], o alfabeto em que o mapa de
// confusões opera.
func KeepAlphanumeric(s string) string {
	return strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '|' {
			return r
		}
		return -1
	}, s)
}

// Date converte uma data DD/MM/AAAA, com qualquer separador, para ISO 8601.
// ok é falso quando não há oito dígitos ou quando a data não existe.
func Date(s string) (iso string, ok bool, transforms []string) {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
	if len(digits) != 8 {
		return "", false, []string{UnreadableDate}
	}
	t, err := time.Parse("02012006", digits)
	if err != nil || t.Year() < 1 {
		return "", false, []string{InvalidDate}
	}
	return fmt.Sprintf("%04d-%02d-%02d", t.Year(), t.Month(), t.Day()), true, []string{DateToISO}
}

var spacesAroundSlash = regexp.MustCompile(`\s*/\s*`)

// Exact canoniza campos comparados por igualdade: maiúsculas, sem acento,
// espaços simples e sem espaço em volta da barra (município/UF).
func Exact(s string) (string, []string) {
	out := CollapseSpaces(StripAccents(Upper(s)))
	out = spacesAroundSlash.ReplaceAllLiteralString(out, "/")
	if out != s {
		return out, []string{CanonicalUppercase}
	}
	return out, []string{}
}

// Apply normaliza o texto conforme o tipo do campo. ok só é falso para datas
// que não puderam ser lidas.
func Apply(kind layout.Kind, text string) (value string, ok bool, transforms []string) {
	switch kind {
	case layout.KindName:
		v, t := Name(text)
		return v, true, t
	case layout.KindIdentifier:
		v, t := Identifier(text)
		return v, true, t
	case layout.KindDate:
		return Date(text)
	default:
		v, t := Exact(text)
		return v, true, t
	}
}
