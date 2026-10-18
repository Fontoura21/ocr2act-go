// Package rules classifica cada campo lido em uma de quatro classes, seguindo
// a seção 8.3 da Entrega Parcial 1: só confusões visuais do mapa sustentam
// "provável erro de OCR", e semelhança fonética nunca gera "compatível".
package rules

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Fontoura21/ocr2act-go/internal/config"
	"github.com/Fontoura21/ocr2act-go/internal/ids"
	"github.com/Fontoura21/ocr2act-go/internal/layout"
	"github.com/Fontoura21/ocr2act-go/internal/normalize"
	"github.com/Fontoura21/ocr2act-go/internal/record"
)

// Class é a decisão sobre um campo.
type Class string

// Classes de decisão, na ordem usada em relatórios e na interface.
const (
	Compatible     Class = "compativel"
	OCRError       Class = "erro_ocr"
	RealDivergence Class = "divergencia_real"
	Review         Class = "revisao"
)

// Classes lista as classes na ordem de exibição.
var Classes = []Class{Compatible, OCRError, RealDivergence, Review}

// Label devolve o rótulo da classe para pessoas.
func (c Class) Label() string {
	switch c {
	case Compatible:
		return "Compatível"
	case OCRError:
		return "Provável erro de OCR"
	case RealDivergence:
		return "Divergência real"
	case Review:
		return "Revisão necessária"
	}
	return string(c)
}

// Decision é a classe atribuída, a regra que decidiu e o motivo legível.
type Decision struct {
	Class  Class  `json:"classe"`
	Rule   string `json:"regra"`
	Reason string `json:"motivo"`
}

// Reading é o que o pipeline leu em um campo.
type Reading struct {
	Raw        string
	Normalized string
	// Valid é falso quando a normalização não produziu valor (data ilegível).
	Valid      bool
	Confidence float64
}

type pair struct{ read, correct string }

// Classifier aplica as regras com os limiares e o mapa de confusões da
// configuração. É imutável e pode ser usado por várias goroutines.
type Classifier struct {
	high      float64
	pairs     map[pair]struct{}
	particles map[string]struct{}
}

// NewClassifier prepara o classificador a partir das regras carregadas.
func NewClassifier(cfg *config.Config) *Classifier {
	c := &Classifier{
		high:      cfg.Engines.Tesseract.High,
		pairs:     make(map[pair]struct{}),
		particles: make(map[string]struct{}, len(cfg.Particles)),
	}
	// Os pares valem nos dois sentidos (8 lido como B e B lido como 8) e
	// também para a leitura em maiúscula, como no protótipo em Python.
	for correct, reads := range cfg.Confusions {
		for _, read := range reads {
			upper := normalize.Upper(read)
			for _, p := range []pair{{read, correct}, {correct, read}, {upper, correct}, {correct, upper}} {
				c.pairs[p] = struct{}{}
			}
		}
	}
	for _, p := range cfg.Particles {
		c.particles[p] = struct{}{}
	}
	return c
}

// Classify decide a classe de um campo a partir da leitura e do registro da
// base. rec nil significa que o registro não foi encontrado.
func (c *Classifier) Classify(f layout.Field, r Reading, rec *record.Record) Decision {
	if rec == nil {
		return Decision{Review, "sem_registro", "registro não encontrado na base de referência"}
	}
	if strings.TrimFunc(r.Raw, normalize.IsSpace) == "" {
		return Decision{Review, "campo_ilegivel", "nenhum texto reconhecido na região"}
	}
	base, _ := rec.Raw(f.Name)
	switch f.Kind {
	case layout.KindIdentifier:
		return c.Identifier(f.Name, r.Normalized, base, r.Confidence)
	case layout.KindName:
		return c.Names(r.Raw, base, r.Confidence)
	case layout.KindDate:
		iso := ""
		if r.Valid {
			iso = r.Normalized
		}
		return c.Dates(r.Raw, iso, base, r.Confidence)
	default:
		canonical, _ := normalize.Exact(rec.Display(f.Name))
		return c.Exact(r.Normalized, canonical, r.Confidence)
	}
}

// Identifier compara CPF ou matrícula. O dígito verificador decide entre erro
// de OCR (leitura com DV inválido explicada pelo mapa) e divergência real
// (leitura com DV válido e confiança alta).
func (c *Classifier) Identifier(field, read, base string, conf float64) Decision {
	valid := ids.ValidCPF
	if field == "matricula" {
		valid = ids.ValidMatricula
	}
	if read == base {
		return Decision{Compatible, "id_igual", "igual ao valor da base"}
	}
	readValid := valid(read)
	if c.explainedByMap(read, base) && !readValid && conf < c.high && valid(base) {
		return Decision{OCRError, "id_confusao_dv_invalido", fmt.Sprintf(
			"%s; todas no mapa de confusão, DV do valor lido inválido, confiança %.2f < %.2f",
			describe(read, base), conf, c.high)}
	}
	sameLength := runeLen(read) == runeLen(base)
	if readValid && conf >= c.high {
		n := "várias"
		if sameLength {
			n = fmt.Sprint(len(diffs(read, base)))
		}
		return Decision{RealDivergence, "id_dv_valido_confianca_alta", fmt.Sprintf(
			"leitura com DV válido e confiança %.2f, difere da base em %s posição(ões)", conf, n)}
	}
	if !sameLength {
		return Decision{Review, "id_tamanho", fmt.Sprintf(
			"tamanho lido %d difere do esperado %d", runeLen(read), runeLen(base))}
	}
	dv := "DV inválido"
	if readValid {
		dv = "DV válido"
	}
	return Decision{Review, "id_sem_evidencia", fmt.Sprintf(
		"%s, confiança %.2f; diferença não explicada só pelo mapa de confusão", dv, conf)}
}

// Names compara nomes de pessoas. Diferenças de grafia com a mesma chave
// fonética vão para revisão, porque variação de grafia tem efeito jurídico.
func (c *Classifier) Names(readRaw, baseRaw string, conf float64) Decision {
	if baseRaw == "" && strings.TrimFunc(readRaw, normalize.IsSpace) == "" {
		return Decision{Compatible, "nome_ausente_nos_dois", "campo vazio no documento e na base"}
	}
	nr, _ := normalize.Name(readRaw)
	nb, _ := normalize.Name(baseRaw)
	if nr == nb {
		if normalize.Upper(strings.TrimFunc(readRaw, normalize.IsSpace)) != normalize.Upper(baseRaw) {
			return Decision{Compatible, "nome_igual_normalizado", "igual após normalização; diferença bruta de acento ou caixa: confira"}
		}
		return Decision{Compatible, "nome_igual", "igual ao valor da base"}
	}

	vr := strings.ReplaceAll(nameWithDigits(readRaw), "RN", "M")
	vb := strings.ReplaceAll(nameWithDigits(baseRaw), "RN", "M")
	if vr == vb || c.explainedByMap(vr, vb) {
		if conf < c.high {
			what := describe(vr, vb)
			if what == "" {
				what = "rn/m"
			}
			return Decision{OCRError, "nome_confusao_visual", fmt.Sprintf(
				"diferença só por confusão visual (%s), confiança %.2f", what, conf)}
		}
		return Decision{Review, "nome_confusao_confianca_alta", fmt.Sprintf(
			"diferença parece confusão visual, mas a confiança é alta (%.2f)", conf)}
	}

	if PhoneticKey(readRaw) == PhoneticKey(baseRaw) {
		return Decision{Review, "nome_variacao_grafia", "mesma chave fonética com grafia diferente (variação de grafia tem efeito jurídico)"}
	}

	tr, tb := c.tokens(nr), c.tokens(nb)
	if conf >= c.high {
		var parts []string
		if onlyBase := missingFrom(tb, tr); len(onlyBase) > 0 {
			parts = append(parts, "na base e não lido: "+strings.Join(onlyBase, " "))
		}
		if onlyRead := missingFrom(tr, tb); len(onlyRead) > 0 {
			parts = append(parts, "lido e não na base: "+strings.Join(onlyRead, " "))
		}
		what := strings.Join(parts, "; ")
		if what == "" {
			what = "ordem dos tokens difere"
		}
		return Decision{RealDivergence, "nome_tokens_confianca_alta", fmt.Sprintf("%s (confiança %.2f)", what, conf)}
	}
	return Decision{Review, "nome_sem_evidencia", fmt.Sprintf("nomes diferem e a confiança é %.2f", conf)}
}

// Dates compara datas. Datas não têm DV, então diferença em um único dígito
// sempre vai para revisão, qualquer que seja a confiança.
func (c *Classifier) Dates(readRaw, readISO, baseISO string, conf float64) Decision {
	if readISO != "" && readISO == baseISO {
		return Decision{Compatible, "data_igual", "igual ao valor da base"}
	}
	baseDigits, ok := dateDigits(baseISO)
	if !ok {
		return Decision{Review, "data_base_invalida", fmt.Sprintf("a data da base não é válida ('%s')", baseISO)}
	}
	if readISO == "" {
		alnum := normalize.KeepAlphanumeric(readRaw)
		if c.explainedByMap(alnum, baseDigits) {
			return Decision{OCRError, "data_invalida_confusao", fmt.Sprintf(
				"data lida inválida; %s (mapa de confusão)", describe(alnum, baseDigits))}
		}
		return Decision{Review, "data_ilegivel", fmt.Sprintf("não foi possível ler uma data válida ('%s')", readRaw)}
	}
	readDigits, _ := dateDigits(readISO)
	n := len(diffs(readDigits, baseDigits))
	if n == 1 {
		return Decision{Review, "data_um_digito", fmt.Sprintf(
			"difere em um único dígito (%s); datas não têm DV", describe(readDigits, baseDigits))}
	}
	if conf >= c.high {
		return Decision{RealDivergence, "data_varios_digitos", fmt.Sprintf(
			"data válida que difere em %d dígitos, confiança %.2f", n, conf)}
	}
	return Decision{Review, "data_sem_evidencia", fmt.Sprintf("difere em %d dígitos com confiança %.2f", n, conf)}
}

// Exact compara campos canonizados por igualdade (hora, sexo, município/UF e
// CNS da serventia).
func (c *Classifier) Exact(read, base string, conf float64) Decision {
	if read == base {
		return Decision{Compatible, "exato_igual", "igual após canonização"}
	}
	if c.explainedByMap(read, base) && conf < c.high {
		return Decision{OCRError, "exato_confusao", describe(read, base) + " (mapa de confusão)"}
	}
	if conf >= c.high {
		return Decision{RealDivergence, "exato_difere_confianca_alta", fmt.Sprintf("difere da base com confiança %.2f", conf)}
	}
	return Decision{Review, "exato_sem_evidencia", fmt.Sprintf("difere da base com confiança %.2f", conf)}
}

// PhoneticKey é uma versão simplificada do BuscaBR (implementação própria e
// provisória): mesma chave indica a mesma pronúncia com grafia diferente.
func PhoneticKey(s string) string {
	n, _ := normalize.Name(s)
	tokens := strings.Fields(n)
	for i, t := range tokens {
		t = strings.TrimPrefix(t, "H")
		tokens[i] = phonetic.Replace(t)
	}
	return strings.Join(tokens, " ")
}

// phonetic aplica as substituições em sequência, na mesma ordem da versão
// Python; um strings.Replacer faria todas de uma vez e mudaria o resultado.
var phonetic = sequentialReplacer{
	{"PH", "F"}, {"TH", "T"}, {"Y", "I"}, {"W", "V"}, {"Z", "S"}, {"SS", "S"},
	{"TT", "T"}, {"LL", "L"}, {"NN", "N"}, {"RR", "R"}, {"MM", "M"}, {"CE", "SE"}, {"CI", "SI"},
}

type sequentialReplacer [][2]string

func (r sequentialReplacer) Replace(s string) string {
	for _, p := range r {
		s = strings.ReplaceAll(s, p[0], p[1])
	}
	return s
}

// nameWithDigits normaliza o nome mantendo dígitos e a barra vertical, para
// que confusões como 0/O apareçam na comparação.
func nameWithDigits(s string) string {
	return normalize.CollapseSpaces(strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '|' || r == ' ' {
			return r
		}
		return ' '
	}, normalize.StripAccents(normalize.Upper(s))))
}

func (c *Classifier) tokens(name string) []string {
	return slices.DeleteFunc(strings.Fields(name), func(t string) bool {
		_, particle := c.particles[t]
		return particle
	})
}

// missingFrom devolve os tokens de a que não aparecem em b, na ordem de a.
func missingFrom(a, b []string) []string {
	var out []string
	for _, t := range a {
		if !slices.Contains(b, t) {
			out = append(out, t)
		}
	}
	return out
}

type diff struct {
	pos         int
	read, other rune
}

// diffs compara posição a posição até o fim da string mais curta.
func diffs(read, base string) []diff {
	r, b := []rune(read), []rune(base)
	var out []diff
	for i := range min(len(r), len(b)) {
		if r[i] != b[i] {
			out = append(out, diff{i, r[i], b[i]})
		}
	}
	return out
}

// explainedByMap informa se read e base têm o mesmo tamanho e todas as
// diferenças são pares do mapa de confusões.
func (c *Classifier) explainedByMap(read, base string) bool {
	if read == base || runeLen(read) != runeLen(base) {
		return false
	}
	for _, d := range diffs(read, base) {
		if _, ok := c.pairs[pair{string(d.read), string(d.other)}]; !ok {
			return false
		}
	}
	return true
}

func describe(read, base string) string {
	ds := diffs(read, base)
	parts := make([]string, len(ds))
	for i, d := range ds {
		parts[i] = fmt.Sprintf("pos %d: '%c' no lugar de '%c'", d.pos+1, d.read, d.other)
	}
	return strings.Join(parts, ", ")
}

// dateDigits converte AAAA-MM-DD em DDMMAAAA.
func dateDigits(iso string) (string, bool) {
	t, err := time.Parse(time.DateOnly, iso)
	if err != nil {
		return "", false
	}
	return t.Format("02012006"), true
}

func runeLen(s string) int {
	return len([]rune(s))
}
