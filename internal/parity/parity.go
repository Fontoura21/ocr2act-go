// Package parity compara dois conjuntos de resultados campo a campo: esta
// implementação contra o protótipo em Python, ou o mesmo pipeline com duas versões do motor de OCR.
package parity

import (
	"slices"

	"github.com/Fontoura21/ocr2act-go/internal/pipeline"
	"github.com/Fontoura21/ocr2act-go/internal/rules"
)

// Side é o que um dos lados leu e decidiu em um campo.
type Side struct {
	Text       string
	Confidence float64
	Class      rules.Class
	Rule       string
}

// Difference é um campo com texto ou decisão diferente entre os lados.
type Difference struct {
	DocID, Field string
	A, B         Side
}

// Report resume a comparação. Os contadores só consideram documentos
// presentes nos dois lados.
type Report struct {
	Documents      int
	OnlyA, OnlyB   []string
	Fields         int
	SameText       int
	SameConfidence int
	// SameDecision conta campos com a mesma classe e a mesma regra.
	SameDecision int
	Differences  []Difference
}

// Compare compara a e b pelo doc_id e, dentro de cada documento, pelo nome
// do campo.
func Compare(a, b []*pipeline.Result) Report {
	byID := make(map[string]*pipeline.Result, len(b))
	for _, r := range b {
		byID[r.DocID] = r
	}
	var rep Report
	seen := make(map[string]bool, len(a))
	for _, ra := range a {
		seen[ra.DocID] = true
		rb, ok := byID[ra.DocID]
		if !ok {
			rep.OnlyA = append(rep.OnlyA, ra.DocID)
			continue
		}
		rep.Documents++
		for _, fa := range ra.Fields {
			fb, ok := rb.Fields.Get(fa.Name)
			if !ok {
				continue
			}
			rep.Fields++
			sameText := fa.Raw == fb.Raw
			sameDecision := fa.Decision.Class == fb.Decision.Class && fa.Decision.Rule == fb.Decision.Rule
			if sameText {
				rep.SameText++
			}
			if fa.Confidence == fb.Confidence {
				rep.SameConfidence++
			}
			if sameDecision {
				rep.SameDecision++
			}
			if !sameText || !sameDecision {
				rep.Differences = append(rep.Differences, Difference{
					DocID: ra.DocID, Field: fa.Name, A: side(fa), B: side(fb),
				})
			}
		}
	}
	for _, rb := range b {
		if !seen[rb.DocID] {
			rep.OnlyB = append(rep.OnlyB, rb.DocID)
		}
	}
	slices.Sort(rep.OnlyA)
	slices.Sort(rep.OnlyB)
	return rep
}

// Identical informa se os dois lados têm os mesmos documentos, textos e
// decisões. Diferenças só de confiança não contam.
func (r Report) Identical() bool {
	return len(r.OnlyA) == 0 && len(r.OnlyB) == 0 && len(r.Differences) == 0
}

func side(f pipeline.Field) Side {
	return Side{Text: f.Raw, Confidence: f.Confidence, Class: f.Decision.Class, Rule: f.Decision.Rule}
}
