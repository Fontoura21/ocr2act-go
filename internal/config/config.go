// Package config carrega os limiares e as regras versionados em
// config/regras.yaml (RNF06), mantidos fora do código para que uma
// recalibração não exija um novo binário.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// Config espelha config/regras.yaml.
type Config struct {
	Version              string              `yaml:"versao"`
	Engines              Engines             `yaml:"motores"`
	Quality              Quality             `yaml:"qualidade"`
	Confusions           map[string][]string `yaml:"confusoes"`
	Particles            []string            `yaml:"particulas"`
	OtherPersonMinFields int                 `yaml:"alerta_outra_pessoa_min_campos"`
}

// Engines agrupa os limiares de cada motor de OCR.
type Engines struct {
	Tesseract Thresholds `yaml:"tesseract"`
}

// Thresholds são os limiares de confiança de um motor, entre 0 e 1.
type Thresholds struct {
	High float64 `yaml:"confianca_alta"`
	Low  float64 `yaml:"confianca_baixa"`
}

// Quality define o limiar mínimo de qualidade da captura.
type Quality struct {
	// MinSharpness é a variância do Laplaciano abaixo da qual o documento
	// inteiro vai para revisão.
	MinSharpness float64 `yaml:"nitidez_minima"`
}

// Load lê e valida o arquivo de regras.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return Parse(data)
}

// Parse decodifica e valida o conteúdo YAML. Chaves desconhecidas são erro,
// para que um limiar com nome digitado errado não seja ignorado em silêncio.
func Parse(data []byte) (*Config, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var c Config
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return &c, nil
}

func (c *Config) validate() error {
	var errs []error
	t := c.Engines.Tesseract
	if t.High <= 0 || t.High > 1 {
		errs = append(errs, fmt.Errorf("motores.tesseract.confianca_alta deve estar em (0, 1], veio %v", t.High))
	}
	if t.Low < 0 || t.Low > t.High {
		errs = append(errs, fmt.Errorf("motores.tesseract.confianca_baixa deve estar em [0, confianca_alta], veio %v", t.Low))
	}
	if c.Quality.MinSharpness < 0 {
		errs = append(errs, fmt.Errorf("qualidade.nitidez_minima não pode ser negativa, veio %v", c.Quality.MinSharpness))
	}
	if c.OtherPersonMinFields < 1 {
		errs = append(errs, fmt.Errorf("alerta_outra_pessoa_min_campos deve ser pelo menos 1, veio %d", c.OtherPersonMinFields))
	}
	// As confusões são comparadas caractere a caractere; uma entrada com mais
	// de um caractere nunca casaria e só esconderia um erro de configuração.
	for _, correct := range slices.Sorted(maps.Keys(c.Confusions)) {
		if utf8.RuneCountInString(correct) != 1 {
			errs = append(errs, fmt.Errorf("confusoes: a chave %q deve ter um único caractere", correct))
		}
		for _, read := range c.Confusions[correct] {
			if utf8.RuneCountInString(read) != 1 {
				errs = append(errs, fmt.Errorf("confusoes[%q]: a leitura %q deve ter um único caractere", correct, read))
			}
		}
	}
	return errors.Join(errs...)
}
