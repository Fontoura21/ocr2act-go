package config

import (
	"strings"
	"testing"
)

func TestLoadRepositoryRules(t *testing.T) {
	c, err := Load("../../config/regras.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Version != "0.1.0" {
		t.Errorf("Version = %q, want 0.1.0", c.Version)
	}
	if c.Engines.Tesseract.High != 0.90 || c.Engines.Tesseract.Low != 0.60 {
		t.Errorf("Tesseract = %+v, want alta 0.90 e baixa 0.60", c.Engines.Tesseract)
	}
	if c.Quality.MinSharpness != 100 {
		t.Errorf("MinSharpness = %v, want 100", c.Quality.MinSharpness)
	}
	if got := c.Confusions["8"]; len(got) != 1 || got[0] != "B" {
		t.Errorf(`Confusions["8"] = %v, want [B]`, got)
	}
	if len(c.Particles) != 6 || c.OtherPersonMinFields != 2 {
		t.Errorf("Particles = %v, OtherPersonMinFields = %d", c.Particles, c.OtherPersonMinFields)
	}
}

const valid = `
versao: "1"
motores:
  tesseract:
    confianca_alta: 0.9
    confianca_baixa: 0.6
qualidade:
  nitidez_minima: 100
confusoes:
  "0": ["O"]
particulas: ["DE"]
alerta_outra_pessoa_min_campos: 2
`

func TestParseRejectsInvalidRules(t *testing.T) {
	tests := []struct {
		name, old, new, wantErr string
	}{
		{"chave desconhecida", "particulas:", "particula_typo: []\nparticulas:", "particula_typo"},
		{"confiança alta acima de 1", "confianca_alta: 0.9", "confianca_alta: 1.5", "confianca_alta"},
		{"confiança baixa acima da alta", "confianca_baixa: 0.6", "confianca_baixa: 0.95", "confianca_baixa"},
		{"nitidez negativa", "nitidez_minima: 100", "nitidez_minima: -1", "nitidez_minima"},
		{"alerta sem campos", "min_campos: 2", "min_campos: 0", "alerta_outra_pessoa_min_campos"},
		{"confusão com dois caracteres", `"0": ["O"]`, `"0": ["OO"]`, `"OO"`},
		{"chave de confusão vazia", `"0": ["O"]`, `"": ["O"]`, `""`},
	}
	if _, err := Parse([]byte(valid)); err != nil {
		t.Fatalf("o caso base deveria ser válido: %v", err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(strings.Replace(valid, tt.old, tt.new, 1)))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Parse() erro = %v, want erro contendo %q", err, tt.wantErr)
			}
		})
	}
}
