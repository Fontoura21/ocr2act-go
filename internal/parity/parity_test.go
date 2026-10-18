package parity

import (
	"testing"

	"github.com/Fontoura21/ocr2act-go/internal/pipeline"
	"github.com/Fontoura21/ocr2act-go/internal/rules"
)

func doc(id string, fields ...pipeline.Field) *pipeline.Result {
	return &pipeline.Result{DocID: id, Fields: fields}
}

func field(name, raw string, conf float64, class rules.Class, rule string) pipeline.Field {
	return pipeline.Field{Name: name, Raw: raw, Confidence: conf, Decision: rules.Decision{Class: class, Rule: rule}}
}

func TestCompare(t *testing.T) {
	a := []*pipeline.Result{
		doc("R001_limpa",
			field("nome", "ANA", 0.95, rules.Compatible, "nome_igual"),
			field("cpf", "529.982.247-25", 0.91, rules.Compatible, "id_igual")),
		doc("R002_forte", field("nome", "J0AO", 0.80, rules.OCRError, "nome_confusao_visual")),
		doc("R003_leve"),
	}
	b := []*pipeline.Result{
		doc("R002_forte", field("nome", "JOAO", 0.95, rules.Compatible, "nome_igual")),
		doc("R001_limpa",
			field("nome", "ANA", 0.95, rules.Compatible, "nome_igual"),
			field("cpf", "529.982.247-25", 0.89, rules.Compatible, "id_igual")),
		doc("R004_limpa"),
	}
	rep := Compare(a, b)
	if rep.Documents != 2 || rep.Fields != 3 {
		t.Fatalf("documentos = %d, campos = %d", rep.Documents, rep.Fields)
	}
	if rep.SameText != 2 || rep.SameConfidence != 1 || rep.SameDecision != 2 {
		t.Errorf("texto = %d, confiança = %d, decisão = %d", rep.SameText, rep.SameConfidence, rep.SameDecision)
	}
	if len(rep.Differences) != 1 || rep.Differences[0].DocID != "R002_forte" || rep.Differences[0].B.Text != "JOAO" {
		t.Errorf("diferenças = %+v", rep.Differences)
	}
	if len(rep.OnlyA) != 1 || rep.OnlyA[0] != "R003_leve" || len(rep.OnlyB) != 1 || rep.OnlyB[0] != "R004_limpa" {
		t.Errorf("só em A = %v, só em B = %v", rep.OnlyA, rep.OnlyB)
	}
	if rep.Identical() {
		t.Error("relatório com diferenças não pode ser idêntico")
	}
}

func TestConfidenceAloneIsNotADifference(t *testing.T) {
	a := []*pipeline.Result{doc("R001_limpa", field("cpf", "1", 0.91, rules.Compatible, "id_igual"))}
	b := []*pipeline.Result{doc("R001_limpa", field("cpf", "1", 0.89, rules.Compatible, "id_igual"))}
	if rep := Compare(a, b); !rep.Identical() || rep.SameConfidence != 0 {
		t.Errorf("relatório = %+v", rep)
	}
}
