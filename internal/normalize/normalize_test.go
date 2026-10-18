package normalize

import (
	"slices"
	"testing"

	"github.com/Fontoura21/ocr2act-go/internal/layout"
)

func TestName(t *testing.T) {
	tests := []struct {
		in, want string
		steps    []string
	}{
		{"MARIA JOSE SILVA", "MARIA JOSE SILVA", nil},
		{"Maria José", "MARIA JOSE", []string{Uppercase, RemoveAccents}},
		{"JOÃO  D'ÁVILA-NETO ", "JOAO D AVILA NETO", []string{RemoveAccents, RemoveSymbols}},
		{"J0AO PEREIRA", "J AO PEREIRA", []string{RemoveSymbols}},
		{"Straße", "STRASSE", []string{Uppercase}},               // mapeamento completo, como o Python
		{"ANA\u00a0PAULA", "ANA PAULA", []string{RemoveAccents}}, // NFKD troca o NBSP por espaço
	}
	for _, tt := range tests {
		got, steps := Name(tt.in)
		if got != tt.want || !slices.Equal(steps, nonNil(tt.steps)) {
			t.Errorf("Name(%q) = %q %v, want %q %v", tt.in, got, steps, tt.want, tt.steps)
		}
	}
}

func TestIdentifier(t *testing.T) {
	if got, steps := Identifier("529.982.247-25"); got != "52998224725" || !slices.Equal(steps, []string{RemovePunctuation}) {
		t.Errorf("Identifier = %q %v", got, steps)
	}
	// Letras e a barra vertical ficam: são evidência de confusão visual.
	if got, _ := Identifier("771088 01 55 2O19|"); got != "77108801552O19|" {
		t.Errorf("Identifier com letras = %q", got)
	}
	if _, steps := Identifier("52998224725"); steps == nil || len(steps) != 0 {
		t.Errorf("sem mudança a lista deve ser vazia e não nula, veio %#v", steps)
	}
}

func TestDate(t *testing.T) {
	tests := []struct {
		in, want string
		ok       bool
		step     string
	}{
		{"15/03/2021", "2021-03-15", true, DateToISO},
		{"15.03.2021", "2021-03-15", true, DateToISO},
		{"29/02/2024", "2024-02-29", true, DateToISO},
		{"29/02/2023", "", false, InvalidDate},
		{"31/04/2021", "", false, InvalidDate},
		{"00/01/2021", "", false, InvalidDate},
		{"01/01/0000", "", false, InvalidDate},
		{"15/O3/2021", "", false, UnreadableDate},
		{"15/03/20", "", false, UnreadableDate},
	}
	for _, tt := range tests {
		got, ok, steps := Date(tt.in)
		if got != tt.want || ok != tt.ok || !slices.Equal(steps, []string{tt.step}) {
			t.Errorf("Date(%q) = %q %v %v, want %q %v [%s]", tt.in, got, ok, steps, tt.want, tt.ok, tt.step)
		}
	}
}

func TestExact(t *testing.T) {
	tests := []struct{ in, want string }{
		{"DA CRUZ DE GOIÁS / BA", "DA CRUZ DE GOIAS/BA"},
		{"  são   paulo/sp ", "SAO PAULO/SP"},
		{"05:23", "05:23"},
		{"A / / B", "A//B"},
	}
	for _, tt := range tests {
		if got, _ := Exact(tt.in); got != tt.want {
			t.Errorf("Exact(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	if _, steps := Exact("MASCULINO"); len(steps) != 0 {
		t.Errorf("Exact sem mudança registrou %v", steps)
	}
}

func TestCollapseSpacesUsesPythonWhitespace(t *testing.T) {
	if got := CollapseSpaces("\x1c A \t\u2003B\u00a0 "); got != "A B" {
		t.Errorf("CollapseSpaces = %q, want %q", got, "A B")
	}
}

func TestApply(t *testing.T) {
	if v, ok, _ := Apply(layout.KindDate, "01/02/2020"); !ok || v != "2020-02-01" {
		t.Errorf("Apply(data) = %q %v", v, ok)
	}
	if _, ok, _ := Apply(layout.KindDate, "sem data"); ok {
		t.Error("Apply(data ilegível) deveria falhar")
	}
	if v, _, _ := Apply(layout.KindIdentifier, "341.316.475-91"); v != "34131647591" {
		t.Errorf("Apply(identificador) = %q", v)
	}
	if v, _, _ := Apply(layout.KindName, "ana"); v != "ANA" {
		t.Errorf("Apply(nome) = %q", v)
	}
	if v, _, _ := Apply(layout.KindExact, "feminino"); v != "FEMININO" {
		t.Errorf("Apply(exato) = %q", v)
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
