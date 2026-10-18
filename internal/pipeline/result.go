package pipeline

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Fontoura21/ocr2act-go/internal/jsonenc"
	"github.com/Fontoura21/ocr2act-go/internal/layout"
	"github.com/Fontoura21/ocr2act-go/internal/ocr"
	"github.com/Fontoura21/ocr2act-go/internal/rules"
)

// Result é o resultado de um documento. O JSON segue o do protótipo em
// Python, para que os resultados dos dois possam ser comparados campo a campo.
type Result struct {
	DocID        string   `json:"doc_id"`
	File         string   `json:"arquivo"`
	SHA256       string   `json:"sha256"`
	Sharpness    float64  `json:"nitidez"`
	QualityOK    bool     `json:"qualidade_ok"`
	DocType      string   `json:"tipo_documental"`
	Lookup       string   `json:"busca_base"`
	MaskedCPF    *string  `json:"cpf_base_mascarado"`
	RulesVersion string   `json:"versao_regras"`
	Fields       Fields   `json:"campos"`
	Alerts       []string `json:"alertas"`
	Seconds      float64  `json:"tempo_s"`
}

// Field é a leitura, a proveniência e a decisão de um campo.
type Field struct {
	Name       string         `json:"-"`
	Label      string         `json:"rotulo"`
	Critical   bool           `json:"critico"`
	Kind       layout.Kind    `json:"tipo"`
	BBox       [4]int         `json:"bbox"`
	Raw        string         `json:"bruto"`
	Normalized *string        `json:"normalizado"`
	Transforms []string       `json:"transformacoes"`
	Confidence float64        `json:"confianca"`
	Words      []ocr.Word     `json:"palavras"`
	Engine     string         `json:"motor"`
	Version    string         `json:"versao"`
	BaseValue  *string        `json:"valor_base"`
	Decision   rules.Decision `json:"decisao"`
}

// Fields são os campos na ordem do template. Em JSON é um objeto indexado
// pelo nome do campo, com a ordem preservada.
type Fields []Field

// Get devolve o campo pelo nome.
func (fs Fields) Get(name string) (Field, bool) {
	i := slices.IndexFunc(fs, func(f Field) bool { return f.Name == name })
	if i < 0 {
		return Field{}, false
	}
	return fs[i], true
}

// MarshalJSON codifica os campos como objeto, na ordem do template.
func (fs Fields) MarshalJSON() ([]byte, error) {
	buf := []byte{'{'}
	for i, f := range fs {
		if i > 0 {
			buf = append(buf, ',')
		}
		key, err := jsonenc.Marshal(f.Name)
		if err != nil {
			return nil, err
		}
		value, err := jsonenc.Marshal(f)
		if err != nil {
			return nil, err
		}
		buf = append(append(append(buf, key...), ':'), value...)
	}
	return append(buf, '}'), nil
}

// UnmarshalJSON decodifica o objeto de campos preservando a ordem.
func (fs *Fields) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return errors.New("pipeline: campos deve ser um objeto JSON")
	}
	out := Fields{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		f := Field{Name: tok.(string)}
		if err := dec.Decode(&f); err != nil {
			return fmt.Errorf("pipeline: campo %s: %w", f.Name, err)
		}
		out = append(out, f)
	}
	*fs = out
	return nil
}

// WriteResult grava o resultado em dir/<doc_id>.json. A escrita passa por um
// arquivo temporário renomeado no fim, para que uma interrupção não deixe um
// resultado pela metade.
func WriteResult(dir string, r *Result) error {
	tmp, err := os.CreateTemp(dir, ".resultado-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // sem efeito depois do Rename
	if err := jsonenc.Write(tmp, r); err != nil {
		return errors.Join(err, tmp.Close())
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, r.DocID+".json"))
}

// ReadResult lê um resultado gravado por esta implementação ou pelo protótipo em Python.
func ReadResult(path string) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Result
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &r, nil
}

// ReadResults lê todos os resultados de dir, em ordem de doc_id.
func ReadResults(dir string) ([]*Result, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []*Result
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		r, err := ReadResult(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b *Result) int { return strings.Compare(a.DocID, b.DocID) })
	return out, nil
}
