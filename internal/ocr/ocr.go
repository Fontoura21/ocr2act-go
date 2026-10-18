// Package ocr lê o texto de uma região da certidão e mede a qualidade da
// captura. O motor padrão é o Tesseract, chamado como processo externo.
package ocr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"strings"
	"unicode"
)

// Engine lê uma linha de texto de uma região em tons de cinza.
type Engine interface {
	Read(ctx context.Context, region *image.Gray) (Reading, error)
}

// Word é uma palavra reconhecida e a confiança do motor, entre 0 e 1. Em JSON
// é o par [texto, confiança], como no protótipo em Python.
type Word struct {
	Text       string
	Confidence float64
}

// MarshalJSON codifica a palavra como [texto, confiança].
func (w Word) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode([2]any{w.Text, w.Confidence}); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// UnmarshalJSON decodifica o par [texto, confiança].
func (w *Word) UnmarshalJSON(data []byte) error {
	var pair []json.RawMessage
	if err := json.Unmarshal(data, &pair); err != nil {
		return err
	}
	if len(pair) != 2 {
		return fmt.Errorf("ocr: palavra deve ser [texto, confiança], veio %s", data)
	}
	if err := json.Unmarshal(pair[0], &w.Text); err != nil {
		return err
	}
	return json.Unmarshal(pair[1], &w.Confidence)
}

// Reading é o resultado da leitura de uma região.
type Reading struct {
	Text string
	// Confidence é a menor confiança entre as palavras com letra ou dígito:
	// um campo é tão confiável quanto a pior palavra que o compõe.
	Confidence float64
	Words      []Word
	Engine     string
	Version    string
}

// NewReading monta a leitura a partir das palavras reconhecidas. Tokens só de
// pontuação entram no texto, mas não na confiança.
func NewReading(words []Word, engine, version string) Reading {
	texts := make([]string, len(words))
	confidence, found := 0.0, false
	for i, w := range words {
		texts[i] = w.Text
		if strings.IndexFunc(w.Text, isAlnum) >= 0 && (!found || w.Confidence < confidence) {
			confidence, found = w.Confidence, true
		}
	}
	if words == nil {
		words = []Word{}
	}
	return Reading{
		Text:       strings.Join(texts, " "),
		Confidence: confidence,
		Words:      words,
		Engine:     engine,
		Version:    version,
	}
}

func isAlnum(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r)
}
