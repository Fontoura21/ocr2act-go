package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Fontoura21/ocr2act-go/internal/dataset"
	"github.com/Fontoura21/ocr2act-go/internal/layout"
	"github.com/Fontoura21/ocr2act-go/internal/pipeline"
	"github.com/Fontoura21/ocr2act-go/internal/record"
	"github.com/Fontoura21/ocr2act-go/internal/rules"
)

var base = record.Record{
	Matricula: "83879701552021100036024008667405", CPF: "34131647591",
	Nome: "APOLLO PORTO SOUZA", DataNascimento: "2021-06-22", HoraNascimento: "05:23",
	Sexo: "MASCULINO", Municipio: "DA CRUZ DE GOIÁS", UF: "BA", Mae: "BELLA ROCHA PORTO",
	Pai: "YAGO BORGES CAMARGO", DataRegistro: "2021-07-05", CNS: "838797",
}

func TestFieldTruth(t *testing.T) {
	withName := func(n string) record.Record { r := base; r.Nome = n; return r }
	tests := []struct {
		name    string
		printed record.Record
		base    *record.Record
		want    Truth
	}{
		{"igual", base, &base, Equal},
		{"sem registro na base", base, nil, NoRecord},
		{"acento omitido é variação legítima", withName("APOLLO PORTO SOUZA"), ptr(withName("APÓLLO PORTO SOUZA")), LegitimateVariation},
		{"Souza e Sousa é variação de grafia", withName("APOLLO PORTO SOUSA"), &base, SpellingVariation},
		{"sobrenome trocado é divergente", withName("APOLLO PORTO LIMA"), &base, Divergent},
	}
	for _, tt := range tests {
		doc := dataset.Doc{Printed: tt.printed, Base: tt.base}
		if got := FieldTruth("nome", doc); got != tt.want {
			t.Errorf("%s: FieldTruth = %s, want %s", tt.name, got, tt.want)
		}
	}
	// Fora dos nomes não existe variação: qualquer diferença é divergência.
	other := base
	other.Sexo = "FEMININO"
	if got := FieldTruth("sexo", dataset.Doc{Printed: other, Base: &base}); got != Divergent {
		t.Errorf("sexo trocado = %s, want divergente", got)
	}
}

// result monta o resultado de um documento lendo exatamente o impresso, com
// todas as decisões compatíveis, e aplica as alterações pedidas.
func result(docID string, printed record.Record, change map[string]pipeline.Field) *pipeline.Result {
	r := &pipeline.Result{DocID: docID}
	for _, f := range layout.Fields() {
		field := pipeline.Field{
			Name: f.Name, Critical: f.Critical, Raw: printed.Display(f.Name),
			Decision: rules.Decision{Class: rules.Compatible},
		}
		if c, ok := change[f.Name]; ok {
			field.Raw, field.Decision = c.Raw, c.Decision
		}
		r.Fields = append(r.Fields, field)
	}
	return r
}

func TestCompute(t *testing.T) {
	divergent := base
	divergent.Nome, divergent.CPF = "APOLLO PORTO LIMA", "52998224725"
	truth := map[string]dataset.Doc{
		"R001_limpa": {DocID: "R001_limpa", Level: "limpa", Split: "calibracao", Printed: base, Base: &base},
		"R002_forte": {DocID: "R002_forte", Level: "forte", Split: "avaliacao", Printed: divergent, Base: &base},
		"R003_leve":  {DocID: "R003_leve", Level: "leve", Split: "avaliacao", Printed: base},
	}
	review := rules.Decision{Class: rules.Review}
	allReview := map[string]pipeline.Field{}
	for _, f := range layout.Fields() {
		allReview[f.Name] = pipeline.Field{Raw: base.Display(f.Name), Decision: review}
	}
	results := []*pipeline.Result{
		result("R003_leve", base, allReview),
		result("R002_forte", divergent, map[string]pipeline.Field{
			// Nome lido com um erro e aceito como compatível: falso compatível crítico.
			"nome": {Raw: "APOLLO PORTO LIMB", Decision: rules.Decision{Class: rules.Compatible}},
			"cpf":  {Raw: "529.982.247-25", Decision: rules.Decision{Class: rules.RealDivergence}},
		}),
		result("R001_limpa", base, nil),
	}
	report, err := Compute(results, truth)
	if err != nil {
		t.Fatal(err)
	}
	if len(report) != 3 || report[0].Name != "calibracao" || report[2].Name != "todos" {
		t.Fatalf("recortes = %+v", report)
	}

	cal := report[0]
	if cal.Fields != 11 || *cal.AcceptableDecisions != 1 || cal.RealDivergenceRecall != nil || cal.FalseCompatibleCritical != 0 {
		t.Errorf("calibração = %+v", cal)
	}
	if row, ok := cal.Matrix.Get("divergente"); !ok || sum2(row) != 0 {
		t.Errorf("a linha divergente deve existir mesmo zerada: %+v", cal.Matrix)
	}
	if _, ok := cal.Matrix.Get("sem_registro"); ok {
		t.Error("sem_registro não ocorre na calibração e não deveria aparecer")
	}

	all := report[2]
	if all.Fields != 33 || all.FalseCompatibleCritical != 1 || all.FalseCompatibleCases[0] != "R002_forte.nome" {
		t.Errorf("todos = %+v", all)
	}
	if *all.RealDivergenceRecall != 0.5 {
		t.Errorf("revocação = %v, want 0.5 (cpf detectado, nome não)", *all.RealDivergenceRecall)
	}
	// Uma decisão inaceitável (o nome divergente aceito) em 33.
	if *all.AcceptableDecisions != 0.97 {
		t.Errorf("decisões aceitáveis = %v, want 0.97", *all.AcceptableDecisions)
	}
	// Ordem das chaves: primeiro aparecimento com os documentos em ordem de doc_id.
	var levels []string
	for _, kv := range all.CERByLevel {
		levels = append(levels, kv.Key)
	}
	if strings.Join(levels, ",") != "limpa,forte,leve" {
		t.Errorf("níveis = %v", levels)
	}
	// Um caractere trocado em "APOLLO PORTO LIMB" sobre todo o texto impresso de R002_forte.
	printed := 0
	for _, f := range layout.Fields() {
		printed += len([]rune(divergent.Display(f.Name)))
	}
	if cer, _ := all.CERByLevel.Get("forte"); cer <= 0 || cer > 1.0/float64(printed)+1e-4 {
		t.Errorf("CER forte = %v", cer)
	}
	if acc, _ := all.AccuracyByField.Get("nome"); acc != 0.667 {
		t.Errorf("acurácia do nome = %v, want 0.667", acc)
	}
}

func TestComputeRequiresTruth(t *testing.T) {
	if _, err := Compute([]*pipeline.Result{{DocID: "R999_limpa"}}, map[string]dataset.Doc{}); err == nil {
		t.Fatal("resultado sem verdade deveria ser erro")
	}
}

func TestWrite(t *testing.T) {
	report := Report{{Name: "todos", Fields: 1, FalseCompatibleCases: []string{},
		CERByLevel: Pairs[float64]{{"limpa", 0}}, Matrix: Pairs[Pairs[int]]{{"igual", Pairs[int]{{"compativel", 1}}}}}}
	path := filepath.Join(t.TempDir(), "metricas.json")
	if err := Write(path, report); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]map[string]any
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("JSON inválido: %v\n%s", err, data)
	}
	if back["todos"]["campos"] != 1.0 || back["todos"]["decisoes_aceitaveis"] != nil {
		t.Errorf("metricas.json = %s", data)
	}
	if !strings.Contains(string(data), `"falso_compativel_critico_casos": []`) {
		t.Errorf("lista vazia deve sair como [], veio:\n%s", data)
	}
}

func TestLevenshtein(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "abc", 0},
		{"", "abc", 3},
		{"kitten", "sitting", 3},
		{"ÁB", "AB", 1}, // conta caracteres, não bytes
		{"529.982.247-25", "529.9B2.247-25", 1},
	}
	for _, tt := range tests {
		if got := Levenshtein(tt.a, tt.b); got != tt.want {
			t.Errorf("Levenshtein(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func sum2(p Pairs[int]) int {
	total := 0
	for _, kv := range p {
		total += kv.Value
	}
	return total
}
