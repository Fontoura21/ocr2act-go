// Package ids valida e formata os identificadores da certidão: CPF e
// matrícula (32 dígitos).
package ids

import "strings"

// CPFCheckDigits calcula os dois dígitos verificadores do CPF a partir dos
// nove primeiros dígitos. Devolve "" se base9 não tiver nove dígitos.
func CPFCheckDigits(base9 string) string {
	if len(base9) != 9 || !isDigits(base9) {
		return ""
	}
	d := make([]int, 0, 10)
	for i := range len(base9) {
		d = append(d, int(base9[i]-'0'))
	}
	d1 := weightedSum(d, 10) * 10 % 11 % 10
	d = append(d, d1)
	d2 := weightedSum(d, 11) * 10 % 11 % 10
	return string([]byte{byte('0' + d1), byte('0' + d2)})
}

// weightedSum soma d[i] * (first - i), com pesos decrescentes a partir de first.
func weightedSum(d []int, first int) int {
	sum := 0
	for i, v := range d {
		sum += v * (first - i)
	}
	return sum
}

// ValidCPF informa se o CPF tem 11 dígitos, não é uma sequência repetida e
// tem os dígitos verificadores corretos.
func ValidCPF(cpf string) bool {
	if len(cpf) != 11 || !isDigits(cpf) || strings.Count(cpf, cpf[:1]) == 11 {
		return false
	}
	return CPFCheckDigits(cpf[:9]) == cpf[9:]
}

// FormatCPF devolve o CPF no formato impresso: 529.982.247-25.
func FormatCPF(cpf string) string {
	return sub(cpf, 0, 3) + "." + sub(cpf, 3, 6) + "." + sub(cpf, 6, 9) + "-" + sub(cpf, 9, len(cpf))
}

// MaskCPF devolve a forma usada em logs (RNF02): ***.982.247-**.
func MaskCPF(cpf string) string {
	if len(cpf) != 11 {
		return "***"
	}
	return "***." + cpf[3:6] + "." + cpf[6:9] + "-**"
}

// Group é um componente da matrícula de certidão.
type Group struct {
	Name string
	Size int
}

// MatriculaGroups são os componentes da matrícula, na ordem em que aparecem:
// CNS(6) acervo(2) serviço(2) ano(4) tipo do livro(1) livro(5) folha(3)
// termo(7) DV(2).
var MatriculaGroups = []Group{
	{"cns", 6}, {"acervo", 2}, {"servico", 2}, {"ano", 4}, {"tipo_livro", 1},
	{"livro", 5}, {"folha", 3}, {"termo", 7}, {"dv", 2},
}

// mod11 calcula um dígito verificador por módulo 11 com pesos decrescentes.
//
// Provisório, como no protótipo em Python: ainda falta conferir com a norma do CNJ e
// testar contra casos montados à mão (Entrega Parcial 1, seção 7.1).
func mod11(digits string) int {
	weight := len(digits) + 2
	sum := 0
	for i := range len(digits) {
		sum += int(digits[i]-'0') * weight
		weight--
	}
	dv := sum * 10 % 11
	if dv == 10 {
		return 1
	}
	return dv
}

// MatriculaCheckDigits calcula os dois dígitos verificadores da matrícula a
// partir dos 30 primeiros dígitos. Devolve "" se body30 não tiver 30 dígitos.
func MatriculaCheckDigits(body30 string) string {
	if len(body30) != 30 || !isDigits(body30) {
		return ""
	}
	d1 := mod11(body30)
	d2 := mod11(body30 + string(rune('0'+d1)))
	return string([]byte{byte('0' + d1), byte('0' + d2)})
}

// ValidMatricula informa se a matrícula tem 32 dígitos e DV correto.
func ValidMatricula(m string) bool {
	return len(m) == 32 && isDigits(m) && MatriculaCheckDigits(m[:30]) == m[30:]
}

// MatriculaParts separa a matrícula nos componentes de MatriculaGroups.
// Uma matrícula curta produz componentes vazios ou incompletos no fim.
func MatriculaParts(m string) []string {
	parts := make([]string, 0, len(MatriculaGroups))
	i := 0
	for _, g := range MatriculaGroups {
		parts = append(parts, sub(m, i, i+g.Size))
		i += g.Size
	}
	return parts
}

// FormatMatricula devolve a matrícula como impressa na certidão, com os
// componentes separados por espaço.
func FormatMatricula(m string) string {
	return strings.Join(MatriculaParts(m), " ")
}

func isDigits(s string) bool {
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}

// sub fatia s como o Python faz: índices fora do intervalo são truncados.
func sub(s string, i, j int) string {
	i, j = min(i, len(s)), min(j, len(s))
	if i >= j {
		return ""
	}
	return s[i:j]
}
