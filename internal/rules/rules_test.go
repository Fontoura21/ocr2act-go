package rules

import (
	"strings"
	"testing"

	"github.com/Fontoura21/ocr2act-go/internal/config"
	"github.com/Fontoura21/ocr2act-go/internal/layout"
	"github.com/Fontoura21/ocr2act-go/internal/record"
)

const (
	cpf      = "52998224725" // CPF de exemplo com DV válido
	otherCPF = "52601815906" // outro CPF válido (o gerar_cpf(Random(7)) do protótipo em Python)
)

func classifier(t *testing.T) *Classifier {
	t.Helper()
	cfg, err := config.Load("../../config/regras.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return NewClassifier(cfg)
}

// Os casos abaixo são os de tests/test_regras.py, montados à mão a partir da
// seção 8.3 da Entrega Parcial 1.

func TestIdentifier(t *testing.T) {
	c := classifier(t)
	withB := strings.Replace(cpf, "8", "B", 1)
	tests := []struct {
		name, read string
		conf       float64
		want       Class
	}{
		{"igual é compatível", cpf, 0.95, Compatible},
		{"B no lugar de 8 com confiança baixa é erro de OCR", withB, 0.55, OCRError},
		{"B no lugar de 8 com confiança alta vai para revisão", withB, 0.97, Review},
		{"CPF de outra pessoa com DV válido é divergência", otherCPF, 0.95, RealDivergence},
		{"DV válido com confiança baixa vai para revisão", otherCPF, 0.70, Review},
		{"tamanho diferente vai para revisão", cpf[:10], 0.95, Review},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.Identifier("cpf", tt.read, cpf, tt.conf); got.Class != tt.want {
				t.Errorf("Identifier(%s) = %+v, want %s", tt.read, got, tt.want)
			}
		})
	}
	d := c.Identifier("cpf", withB, cpf, 0.55)
	if want := "pos 5: 'B' no lugar de '8'; todas no mapa de confusão, DV do valor lido inválido, confiança 0.55 < 0.90"; d.Reason != want {
		t.Errorf("motivo = %q, want %q", d.Reason, want)
	}
}

func TestIdentifierMatricula(t *testing.T) {
	c := classifier(t)
	m := "83879701552021100036024008667405"
	read := strings.Replace(m, "0", "O", 1)
	if got := c.Identifier("matricula", read, m, 0.60); got.Class != OCRError {
		t.Errorf("matrícula com O no lugar de 0 = %+v, want erro_ocr", got)
	}
	if got := c.Identifier("matricula", m[:31]+"6", m, 0.95); got.Class != Review || got.Rule != "id_sem_evidencia" {
		t.Errorf("matrícula com DV inválido e confiança alta = %+v, want revisão", got)
	}
}

func TestNames(t *testing.T) {
	c := classifier(t)
	tests := []struct {
		name, read, base string
		conf             float64
		want             Class
		rule             string
	}{
		{"acento omitido é compatível com aviso", "MARIA JOSE SILVA", "MARIA JOSÉ SILVA", 0.95, Compatible, "nome_igual_normalizado"},
		{"igual é compatível", "ANA PAULA", "ANA PAULA", 0.95, Compatible, "nome_igual"},
		{"Luiz e Luis vão para revisão", "LUIS SOUZA", "LUIZ SOUZA", 0.98, Review, "nome_variacao_grafia"},
		{"sobrenome trocado com confiança alta é divergência", "ANA PAULA COSTA", "ANA PAULA MOREIRA", 0.95, RealDivergence, "nome_tokens_confianca_alta"},
		{"zero no lugar de O com confiança baixa é erro de OCR", "J0AO PEREIRA", "JOAO PEREIRA", 0.50, OCRError, "nome_confusao_visual"},
		{"confusão visual com confiança alta vai para revisão", "J0AO PEREIRA", "JOAO PEREIRA", 0.95, Review, "nome_confusao_confianca_alta"},
		{"rn lido no lugar de m", "ANA MARIA", "ANA RNARIA", 0.50, OCRError, "nome_confusao_visual"},
		{"nomes diferentes com confiança baixa vão para revisão", "ANA COSTA", "BEATRIZ LIMA", 0.50, Review, "nome_sem_evidencia"},
		{"vazio nos dois é compatível", "  ", "", 0.0, Compatible, "nome_ausente_nos_dois"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.Names(tt.read, tt.base, tt.conf)
			if got.Class != tt.want || got.Rule != tt.rule {
				t.Errorf("Names(%q, %q) = %+v, want %s/%s", tt.read, tt.base, got, tt.want, tt.rule)
			}
		})
	}
	d := c.Names("ANA PAULA COSTA", "ANA PAULA MOREIRA", 0.95)
	if want := "na base e não lido: MOREIRA; lido e não na base: COSTA (confiança 0.95)"; d.Reason != want {
		t.Errorf("motivo = %q, want %q", d.Reason, want)
	}
	if d := c.Names("MARIA JOSE SILVA", "MARIA JOSÉ SILVA", 0.95); !strings.Contains(d.Reason, "acento") {
		t.Errorf("o motivo deveria citar o acento: %q", d.Reason)
	}
}

func TestHighSimilarityIsNeverCompatible(t *testing.T) {
	c := classifier(t)
	if got := c.Names("ANA PAULA MOREIRAS", "ANA PAULA MOREIRA", 0.99); got.Class == Compatible {
		t.Errorf("similaridade alta sem igualdade não pode ser compatível: %+v", got)
	}
}

func TestPhoneticKey(t *testing.T) {
	tests := []struct{ a, b string }{
		{"LUIZ", "LUIS"},
		{"THIAGO", "TIAGO"},
		{"WAGNER", "VAGNER"},
		{"MATTOS", "MATOS"},
		{"HELENA", "ELENA"},
	}
	for _, tt := range tests {
		if PhoneticKey(tt.a) != PhoneticKey(tt.b) {
			t.Errorf("PhoneticKey(%s) = %s, PhoneticKey(%s) = %s; deveriam ser iguais", tt.a, PhoneticKey(tt.a), tt.b, PhoneticKey(tt.b))
		}
	}
	// As trocas são em sequência: SS vira S depois que Z virou S.
	if got := PhoneticKey("SOUZZA"); got != "SOUSA" {
		t.Errorf("PhoneticKey(SOUZZA) = %s, want SOUSA", got)
	}
}

func TestDates(t *testing.T) {
	c := classifier(t)
	tests := []struct {
		raw, iso string
		conf     float64
		want     Class
	}{
		{"15/03/2021", "2021-03-15", 0.95, Compatible},
		{"15/03/2027", "2027-03-15", 0.99, Review},         // um dígito: revisão em qualquer confiança
		{"02/11/2018", "2018-11-02", 0.95, RealDivergence}, // vários dígitos, confiança alta
		{"02/11/2018", "2018-11-02", 0.70, Review},         // vários dígitos, confiança baixa
		{"15/O3/2021", "", 0.50, OCRError},                 // letra O no lugar de 0
		{"15/03/20", "", 0.95, Review},                     // ilegível
	}
	for _, tt := range tests {
		if got := c.Dates(tt.raw, tt.iso, "2021-03-15", tt.conf); got.Class != tt.want {
			t.Errorf("Dates(%q) = %+v, want %s", tt.raw, got, tt.want)
		}
	}
	if got := c.Dates("15/03/2021", "2021-03-15", "15/03/2021", 0.95); got.Rule != "data_base_invalida" {
		t.Errorf("data da base inválida = %+v", got)
	}
}

func TestExact(t *testing.T) {
	c := classifier(t)
	tests := []struct {
		read, base string
		conf       float64
		want       Class
	}{
		{"MASCULINO", "MASCULINO", 0.80, Compatible},
		{"O5:23", "05:23", 0.80, OCRError},
		{"O5:23", "05:23", 0.95, RealDivergence},
		{"FEMININO", "MASCULINO", 0.95, RealDivergence},
		{"FEMININO", "MASCULINO", 0.70, Review},
	}
	for _, tt := range tests {
		if got := c.Exact(tt.read, tt.base, tt.conf); got.Class != tt.want {
			t.Errorf("Exact(%q, %q, %.2f) = %+v, want %s", tt.read, tt.base, tt.conf, got, tt.want)
		}
	}
}

func TestConfusionPairs(t *testing.T) {
	c := classifier(t)
	for _, p := range []struct {
		read, base string
		want       bool
	}{
		{"B", "8", true},
		{"8", "B", true}, // nos dois sentidos
		{"L", "1", true}, // "l" do mapa também vale em maiúscula
		{"o", "0", true},
		{"X", "0", false},
		{"0", "0", false}, // iguais não são confusão
		{"00", "0", false},
	} {
		if got := c.explainedByMap(p.read, p.base); got != p.want {
			t.Errorf("explainedByMap(%q, %q) = %v, want %v", p.read, p.base, got, p.want)
		}
	}
}

func TestClassify(t *testing.T) {
	c := classifier(t)
	rec := &record.Record{
		CPF: cpf, Nome: "ANA PAULA", DataNascimento: "2021-03-15", Sexo: "FEMININO",
		Municipio: "SÃO PAULO", UF: "SP",
	}
	field := func(name string) layout.Field {
		f, _ := layout.ByName(name)
		return f
	}
	tests := []struct {
		field string
		r     Reading
		rec   *record.Record
		rule  string
	}{
		{"cpf", Reading{Raw: "529.982.247-25", Normalized: cpf, Valid: true, Confidence: 0.95}, nil, "sem_registro"},
		{"cpf", Reading{Raw: " ", Valid: true}, rec, "campo_ilegivel"},
		{"cpf", Reading{Raw: "529.982.247-25", Normalized: cpf, Valid: true, Confidence: 0.95}, rec, "id_igual"},
		{"nome", Reading{Raw: "ANA PAULA", Normalized: "ANA PAULA", Valid: true, Confidence: 0.95}, rec, "nome_igual"},
		{"data_nascimento", Reading{Raw: "15/03/2021", Normalized: "2021-03-15", Valid: true, Confidence: 0.95}, rec, "data_igual"},
		{"data_nascimento", Reading{Raw: "15/O3/2021", Confidence: 0.50}, rec, "data_invalida_confusao"},
		{"municipio_uf", Reading{Raw: "SAO PAULO / SP", Normalized: "SAO PAULO/SP", Valid: true, Confidence: 0.95}, rec, "exato_igual"},
		{"sexo", Reading{Raw: "MASCULINO", Normalized: "MASCULINO", Valid: true, Confidence: 0.95}, rec, "exato_difere_confianca_alta"},
	}
	for _, tt := range tests {
		if got := c.Classify(field(tt.field), tt.r, tt.rec); got.Rule != tt.rule {
			t.Errorf("Classify(%s, %q) = %+v, want regra %s", tt.field, tt.r.Raw, got, tt.rule)
		}
	}
}

func TestLabels(t *testing.T) {
	for _, c := range Classes {
		if c.Label() == string(c) {
			t.Errorf("classe %s sem rótulo", c)
		}
	}
}
