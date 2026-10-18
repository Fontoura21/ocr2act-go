package ids

import (
	"strings"
	"testing"
)

const cpf = "52998224725" // CPF de exemplo com DV válido

func TestCPF(t *testing.T) {
	tests := []struct {
		cpf  string
		want bool
	}{
		{cpf, true},
		{"52601815906", true},
		{"52998224726", false}, // DV errado
		{"11111111111", false}, // sequência repetida, mesmo com DV "válido"
		{"5299822472", false},  // curto
		{"5299822472B", false}, // letra no lugar de dígito
		{"", false},
	}
	for _, tt := range tests {
		if got := ValidCPF(tt.cpf); got != tt.want {
			t.Errorf("ValidCPF(%q) = %v, want %v", tt.cpf, got, tt.want)
		}
	}
	if got := CPFCheckDigits("529982247"); got != "25" {
		t.Errorf("CPFCheckDigits = %q, want 25", got)
	}
	if got := CPFCheckDigits("12345678"); got != "" {
		t.Errorf("CPFCheckDigits com 8 dígitos = %q, want vazio", got)
	}
	if got := FormatCPF(cpf); got != "529.982.247-25" {
		t.Errorf("FormatCPF = %q", got)
	}
	if got := MaskCPF(cpf); got != "***.982.247-**" {
		t.Errorf("MaskCPF = %q", got)
	}
	if got := MaskCPF("123"); got != "***" {
		t.Errorf("MaskCPF curto = %q", got)
	}
}

func TestMatriculaCheckDigits(t *testing.T) {
	// Vetores calculados pelo protótipo em Python (ocr2act.ids.matricula_dv).
	tests := []struct{ body, want string }{
		{"123456015520191291417776317066", "51"},
		{"000000000000000000000000000000", "00"},
		{"999999999999999999999999999999", "56"},
		{"838797015520211000360240086674", "05"},
		{"100000000000000000000000063352", "19"}, // primeiro DV dá 10 e vira 1
	}
	for _, tt := range tests {
		if got := MatriculaCheckDigits(tt.body); got != tt.want {
			t.Errorf("MatriculaCheckDigits(%s) = %q, want %q", tt.body, got, tt.want)
		}
	}
	if got := MatriculaCheckDigits("123"); got != "" {
		t.Errorf("MatriculaCheckDigits curto = %q, want vazio", got)
	}
}

func TestMatricula(t *testing.T) {
	m := "83879701552021100036024008667405"
	if !ValidMatricula(m) {
		t.Fatalf("ValidMatricula(%s) = false", m)
	}
	broken := m[:31] + "6"
	if ValidMatricula(broken) {
		t.Errorf("ValidMatricula com DV alterado = true")
	}
	if ValidMatricula(m[:31]) || ValidMatricula(strings.Replace(m, "8", "B", 1)) {
		t.Errorf("ValidMatricula aceitou matrícula curta ou com letra")
	}
	parts := MatriculaParts(m)
	if len(parts) != len(MatriculaGroups) || parts[0] != "838797" || parts[8] != "05" {
		t.Errorf("MatriculaParts = %v", parts)
	}
	if got, want := FormatMatricula(m), "838797 01 55 2021 1 00036 024 0086674 05"; got != want {
		t.Errorf("FormatMatricula = %q, want %q", got, want)
	}
	// Como o fatiamento do Python, uma matrícula curta não estoura.
	if got := FormatMatricula("8387970155"); got != "838797 01 55      " {
		t.Errorf("FormatMatricula curta = %q", got)
	}
}
