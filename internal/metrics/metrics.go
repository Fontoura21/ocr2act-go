// Package metrics mede as decisões contra a verdade de referência (RF12).
// A verdade de cada campo vem do gerador: valor impresso, valor da base e tipo
// de alteração plantada.
package metrics

import (
	"bytes"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/Fontoura21/ocr2act-go/internal/dataset"
	"github.com/Fontoura21/ocr2act-go/internal/jsonenc"
	"github.com/Fontoura21/ocr2act-go/internal/layout"
	"github.com/Fontoura21/ocr2act-go/internal/normalize"
	"github.com/Fontoura21/ocr2act-go/internal/pipeline"
	"github.com/Fontoura21/ocr2act-go/internal/round"
	"github.com/Fontoura21/ocr2act-go/internal/rules"
)

// Truth é a situação verdadeira de um campo.
type Truth string

// Situações verdadeiras, na ordem da matriz de confusão.
const (
	Equal               Truth = "igual"
	LegitimateVariation Truth = "variacao_legitima"
	SpellingVariation   Truth = "variacao_grafia"
	Divergent           Truth = "divergente"
	NoRecord            Truth = "sem_registro"
)

// Truths lista as situações na ordem das linhas da matriz.
var Truths = []Truth{Equal, LegitimateVariation, SpellingVariation, Divergent, NoRecord}

// acceptable são as classes corretas para cada situação verdadeira.
var acceptable = map[Truth][]rules.Class{
	Equal:               {rules.Compatible, rules.OCRError, rules.Review},
	LegitimateVariation: {rules.Compatible, rules.Review},
	SpellingVariation:   {rules.Review},
	Divergent:           {rules.RealDivergence, rules.Review},
	NoRecord:            {rules.Review},
}

// Splits são os recortes calculados: o conjunto é dividido por registro em
// calibração e avaliação, e "todos" junta os dois.
var Splits = []string{"calibracao", "avaliacao", "todos"}

// FieldTruth classifica a situação verdadeira de um campo comparando o valor
// impresso com o da base. Nomes iguais após a normalização são variação
// legítima; com a mesma chave fonética, variação de grafia.
func FieldTruth(field string, doc dataset.Doc) Truth {
	if doc.Base == nil {
		return NoRecord
	}
	printed, base := doc.Printed.Display(field), doc.Base.Display(field)
	if printed == base {
		return Equal
	}
	if f, ok := layout.ByName(field); ok && f.Kind == layout.KindName {
		np, _ := normalize.Name(printed)
		nb, _ := normalize.Name(base)
		if np == nb {
			return LegitimateVariation
		}
		if rules.PhoneticKey(printed) == rules.PhoneticKey(base) {
			return SpellingVariation
		}
	}
	return Divergent
}

// Split são as métricas de um recorte. Os campos opcionais ficam nulos quando
// o recorte não tem casos para medir.
type Split struct {
	Name                    string            `json:"-"`
	Fields                  int               `json:"campos"`
	AcceptableDecisions     *float64          `json:"decisoes_aceitaveis"`
	FalseCompatibleCritical int               `json:"falso_compativel_critico"`
	FalseCompatibleCases    []string          `json:"falso_compativel_critico_casos"`
	RealDivergenceRecall    *float64          `json:"revocacao_divergencia_real"`
	CERByLevel              Pairs[float64]    `json:"cer_por_nivel"`
	AccuracyByField         Pairs[float64]    `json:"acuracia_por_campo"`
	Matrix                  Pairs[Pairs[int]] `json:"matriz"`
}

// Report são os recortes na ordem de Splits. Em JSON é um objeto indexado
// pelo nome do recorte.
type Report []Split

// MarshalJSON codifica o relatório como objeto, na ordem dos recortes.
func (r Report) MarshalJSON() ([]byte, error) {
	p := make(Pairs[Split], len(r))
	for i, s := range r {
		p[i] = Pair[Split]{Key: s.Name, Value: s}
	}
	return p.MarshalJSON()
}

// Compute calcula as métricas dos resultados contra a verdade. Todo resultado
// precisa ter verdade.
func Compute(results []*pipeline.Result, truth map[string]dataset.Doc) (Report, error) {
	sorted := slices.Clone(results)
	slices.SortFunc(sorted, func(a, b *pipeline.Result) int { return strings.Compare(a.DocID, b.DocID) })
	for _, r := range sorted {
		if _, ok := truth[r.DocID]; !ok {
			return nil, fmt.Errorf("métricas: %s não está na verdade de referência", r.DocID)
		}
	}
	report := make(Report, 0, len(Splits))
	for _, name := range Splits {
		report = append(report, computeSplit(name, sorted, truth))
	}
	return report, nil
}

func computeSplit(name string, results []*pipeline.Result, truth map[string]dataset.Doc) Split {
	matrix := map[Truth]map[rules.Class]int{}
	var cerNum, cerDen, hits, seen counter
	s := Split{Name: name, FalseCompatibleCases: []string{}}
	accepted := 0
	for _, r := range results {
		doc := truth[r.DocID]
		if name != "todos" && doc.Split != name {
			continue
		}
		for _, f := range r.Fields {
			v, class := FieldTruth(f.Name, doc), f.Decision.Class
			if matrix[v] == nil {
				matrix[v] = map[rules.Class]int{}
			}
			matrix[v][class]++
			s.Fields++
			if slices.Contains(acceptable[v], class) {
				accepted++
			}
			printed := doc.Printed.Display(f.Name)
			cerNum.add(doc.Level, Levenshtein(f.Raw, printed))
			cerDen.add(doc.Level, len([]rune(printed)))
			seen.add(f.Name, 1)
			hit := 0
			if f.Raw == printed {
				hit = 1
			}
			hits.add(f.Name, hit)
			if f.Critical && v == Divergent && (class == rules.Compatible || class == rules.OCRError) {
				s.FalseCompatibleCases = append(s.FalseCompatibleCases, r.DocID+"."+f.Name)
			}
		}
	}
	s.FalseCompatibleCritical = len(s.FalseCompatibleCases)
	if s.Fields > 0 {
		s.AcceptableDecisions = ptr(round.To(float64(accepted)/float64(s.Fields), 3))
	}
	if div := matrix[Divergent]; sum(div) > 0 {
		s.RealDivergenceRecall = ptr(round.To(float64(div[rules.RealDivergence])/float64(sum(div)), 3))
	}
	s.CERByLevel = Pairs[float64]{}
	for _, level := range cerDen.keys {
		s.CERByLevel = append(s.CERByLevel, Pair[float64]{level, round.To(float64(cerNum.n[level])/float64(cerDen.n[level]), 4)})
	}
	s.AccuracyByField = Pairs[float64]{}
	for _, field := range seen.keys {
		s.AccuracyByField = append(s.AccuracyByField, Pair[float64]{field, round.To(float64(hits.n[field])/float64(seen.n[field]), 3)})
	}
	// A linha "divergente" sai sempre, mesmo zerada, porque é a da métrica
	// principal; as outras só aparecem quando o recorte tem o caso.
	s.Matrix = Pairs[Pairs[int]]{}
	for _, v := range Truths {
		if _, ok := matrix[v]; !ok && v != Divergent {
			continue
		}
		row := make(Pairs[int], len(rules.Classes))
		for i, c := range rules.Classes {
			row[i] = Pair[int]{string(c), matrix[v][c]}
		}
		s.Matrix = append(s.Matrix, Pair[Pairs[int]]{string(v), row})
	}
	return s
}

// Write grava o relatório em JSON.
func Write(path string, r Report) error {
	var buf bytes.Buffer
	if err := jsonenc.Write(&buf, r); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// Levenshtein é a distância de edição entre a e b, contada em caracteres
// Unicode e com custo 1 para inserção, remoção e troca.
func Levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(rb)]
}

// counter soma valores por chave lembrando a ordem do primeiro aparecimento,
// que é a ordem das chaves no JSON.
type counter struct {
	keys []string
	n    map[string]int
}

func (c *counter) add(key string, v int) {
	if c.n == nil {
		c.n = map[string]int{}
	}
	if _, ok := c.n[key]; !ok {
		c.keys = append(c.keys, key)
	}
	c.n[key] += v
}

func sum(m map[rules.Class]int) int {
	total := 0
	for _, v := range m {
		total += v
	}
	return total
}

func ptr[T any](v T) *T { return &v }
