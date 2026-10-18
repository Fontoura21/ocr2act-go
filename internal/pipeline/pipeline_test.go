package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Fontoura21/ocr2act-go/internal/config"
	"github.com/Fontoura21/ocr2act-go/internal/layout"
	"github.com/Fontoura21/ocr2act-go/internal/ocr"
	"github.com/Fontoura21/ocr2act-go/internal/record"
	"github.com/Fontoura21/ocr2act-go/internal/reference"
	"github.com/Fontoura21/ocr2act-go/internal/rules"
)

var rec = record.Record{
	Matricula: "83879701552021100036024008667405", CPF: "34131647591",
	Nome: "APOLLO PORTO SOUZA", DataNascimento: "2021-06-22", HoraNascimento: "05:23",
	Sexo: "MASCULINO", Municipio: "DA CRUZ DE GOIÁS", UF: "BA", Mae: "BELLA ROCHA PORTO",
	Pai: "YAGO BORGES CAMARGO", DataRegistro: "2021-07-05", CNS: "838797",
}

// fakeEngine devolve uma leitura fixa por região, identificada pelo retângulo.
type fakeEngine struct {
	mu      sync.Mutex
	byRect  map[image.Rectangle]ocr.Reading
	regions int
}

func (e *fakeEngine) Read(ctx context.Context, region *image.Gray) (ocr.Reading, error) {
	if err := ctx.Err(); err != nil {
		return ocr.Reading{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.regions++
	r, ok := e.byRect[region.Rect]
	if !ok {
		return ocr.Reading{}, fmt.Errorf("região inesperada %v", region.Rect)
	}
	return r, nil
}

// engineFor lê o valor impresso de printed em cada campo, com a confiança
// dada, sobrescrevendo os campos de override.
func engineFor(printed record.Record, conf float64, override map[string]string) *fakeEngine {
	e := &fakeEngine{byRect: map[image.Rectangle]ocr.Reading{}}
	for _, f := range layout.Fields() {
		text := printed.Display(f.Name)
		if v, ok := override[f.Name]; ok {
			text = v
		}
		words := []ocr.Word{{Text: text, Confidence: conf}}
		box := f.Region(image.Pt(layout.PageWidth, layout.PageHeight), layout.DefaultInset)
		e.byRect[box] = ocr.NewReading(words, "fake", "1.0")
	}
	return e
}

type source []reference.Entry

func (s source) ByMatricula(_ context.Context, m string) ([]reference.Entry, error) {
	var out []reference.Entry
	for _, e := range s {
		if e.Matricula == m {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s source) ByCPF(_ context.Context, cpf string) ([]reference.Entry, error) {
	var out []reference.Entry
	for _, e := range s {
		if e.CPF == cpf {
			out = append(out, e)
		}
	}
	return out, nil
}

func loadConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load("../../config/regras.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// page devolve uma página PNG do tamanho de referência. Listrada, a nitidez
// fica bem acima do limiar; lisa, fica em zero.
func page(t *testing.T, striped bool) []byte {
	t.Helper()
	if striped {
		return stripedPage()
	}
	return flatPage()
}

var (
	stripedPage = sync.OnceValue(func() []byte { return encodePage(true) })
	flatPage    = sync.OnceValue(func() []byte { return encodePage(false) })
)

func encodePage(striped bool) []byte {
	img := image.NewGray(image.Rect(0, 0, layout.PageWidth, layout.PageHeight))
	for i := range img.Pix {
		img.Pix[i] = 0xff
		if striped && i%2 == 0 {
			img.Pix[i] = 0
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err) // escrever num bytes.Buffer não falha
	}
	return buf.Bytes()
}

func TestProcessCompatibleDocument(t *testing.T) {
	p := New(loadConfig(t), engineFor(rec, 0.95, nil), source{{Record: rec, Synthetic: 1}})
	r, err := p.ProcessImage(context.Background(), "R001_limpa", "R001_limpa.png", page(t, true))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Fields) != 11 || r.Lookup != reference.ViaMatricula || len(r.Alerts) != 0 || !r.QualityOK {
		t.Fatalf("resultado = %+v", r)
	}
	for _, f := range r.Fields {
		if f.Decision.Class != rules.Compatible {
			t.Errorf("%s = %+v, want compatível", f.Name, f.Decision)
		}
	}
	nome, _ := r.Fields.Get("nome")
	if nome.BBox != [4]int{85, 361, 1155, 405} || *nome.BaseValue != "APOLLO PORTO SOUZA" {
		t.Errorf("nome = %+v", nome)
	}
	data, _ := r.Fields.Get("data_nascimento")
	if *data.Normalized != "2021-06-22" {
		t.Errorf("data normalizada = %q", *data.Normalized)
	}
	if *r.MaskedCPF != "***.316.475-**" || r.DocType != "nascimento" || r.RulesVersion != "0.1.0" || len(r.SHA256) != 64 {
		t.Errorf("metadados = %+v", r)
	}
}

func TestProcessFlagsDocumentOfAnotherPerson(t *testing.T) {
	override := map[string]string{"nome": "ANA PAULA COSTA", "cpf": "529.982.247-25"}
	p := New(loadConfig(t), engineFor(rec, 0.95, override), source{{Record: rec}})
	r, err := p.ProcessImage(context.Background(), "R009_limpa", "R009_limpa.png", page(t, true))
	if err != nil {
		t.Fatal(err)
	}
	want := "Possível documento de outra pessoa: 2 campos críticos divergem (nome, cpf). Documento inteiro vai para revisão."
	if len(r.Alerts) != 1 || r.Alerts[0] != want {
		t.Errorf("alertas = %q, want %q", r.Alerts, want)
	}
}

func TestProcessSendsBlurryCaptureToReview(t *testing.T) {
	p := New(loadConfig(t), engineFor(rec, 0.95, nil), source{{Record: rec}})
	r, err := p.ProcessImage(context.Background(), "R001_forte", "R001_forte.jpg", page(t, false))
	if err != nil {
		t.Fatal(err)
	}
	if r.QualityOK || r.Sharpness != 0 {
		t.Fatalf("nitidez = %v, qualidade_ok = %v", r.Sharpness, r.QualityOK)
	}
	for _, f := range r.Fields {
		if f.Decision.Rule != "qualidade_baixa" || f.Decision.Reason != "captura abaixo do limiar de nitidez (0)" {
			t.Errorf("%s = %+v, want qualidade_baixa", f.Name, f.Decision)
		}
	}
}

func TestProcessWithoutReferenceRecord(t *testing.T) {
	p := New(loadConfig(t), engineFor(rec, 0.95, map[string]string{"data_nascimento": "22/06/20"}), source{})
	r, err := p.ProcessImage(context.Background(), "R012_limpa", "R012_limpa.png", page(t, true))
	if err != nil {
		t.Fatal(err)
	}
	if r.Lookup != reference.ViaNotFound || r.MaskedCPF != nil {
		t.Errorf("busca = %q, cpf mascarado = %v", r.Lookup, r.MaskedCPF)
	}
	if len(r.Alerts) != 1 || r.Alerts[0] != "Registro não localizado na base (busca: nao_encontrado)." {
		t.Errorf("alertas = %q", r.Alerts)
	}
	for _, f := range r.Fields {
		if f.Decision.Rule != "sem_registro" || f.BaseValue != nil {
			t.Errorf("%s = %+v", f.Name, f)
		}
	}
	data, _ := r.Fields.Get("data_nascimento")
	if data.Normalized != nil || data.Transforms[0] != "data_ilegivel" {
		t.Errorf("data ilegível = %+v", data)
	}
}

func TestProcessRejectsUnreadableImage(t *testing.T) {
	p := New(loadConfig(t), engineFor(rec, 0.95, nil), source{})
	if _, err := p.ProcessImage(context.Background(), "x", "x.png", []byte("não é imagem")); err == nil {
		t.Fatal("imagem inválida deveria falhar")
	}
}

func TestResultJSONRoundTrip(t *testing.T) {
	p := New(loadConfig(t), engineFor(rec, 0.95, map[string]string{"data_registro": "sem data"}), source{{Record: rec}})
	r, err := p.ProcessImage(context.Background(), "R001_limpa", "R001_limpa.png", page(t, true))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := WriteResult(dir, r); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "R001_limpa.json"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{`"normalizado": null`, `"transformacoes": []`, `"palavras": [`, `"MUNICÍPIO DE NASCIMENTO / UF"`} {
		if !strings.Contains(text, want) {
			t.Errorf("JSON sem %s", want)
		}
	}
	if strings.Index(text, `"nome": {`) > strings.Index(text, `"cns": {`) {
		t.Error("os campos devem sair na ordem do template")
	}
	back, err := ReadResult(filepath.Join(dir, "R001_limpa.json"))
	if err != nil {
		t.Fatal(err)
	}
	again, _ := json.Marshal(back)
	first, _ := json.Marshal(r)
	if !bytes.Equal(first, again) {
		t.Errorf("ida e volta mudou o resultado:\n%s\n%s", first, again)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("sobrou arquivo temporário: %v", entries)
	}
}

func TestProcessAll(t *testing.T) {
	dir := t.TempDir()
	img := page(t, true)
	for i := 1; i <= 6; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("R%03d_limpa.png", i)), img, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "R007_limpa.png"), []byte("corrompido"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths, err := Documents(dir)
	if err != nil || len(paths) != 7 {
		t.Fatalf("Documents = %v, %v", paths, err)
	}
	engine := engineFor(rec, 0.95, nil)
	p := New(loadConfig(t), engine, source{{Record: rec}})
	got := map[string]bool{}
	failed := 0
	for o := range p.ProcessAll(context.Background(), paths, 3) {
		if o.Err != nil {
			failed++
			continue
		}
		got[o.Result.DocID] = true
	}
	if len(got) != 6 || failed != 1 || !got["R006_limpa"] {
		t.Errorf("processados = %v, falhas = %d", got, failed)
	}
	if engine.regions != 6*11 {
		t.Errorf("regiões lidas = %d, want %d", engine.regions, 6*11)
	}
}

func TestProcessAllStopsWhenCancelled(t *testing.T) {
	dir := t.TempDir()
	for i := range 20 {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("R%03d_leve.png", i)), page(t, true), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	paths, _ := Documents(dir)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := New(loadConfig(t), engineFor(rec, 0.95, nil), source{{Record: rec}})
	done := make(chan int)
	go func() {
		n := 0
		for o := range p.ProcessAll(ctx, paths, 4) {
			if o.Err == nil {
				n++
			}
		}
		done <- n
	}()
	select {
	case n := <-done:
		if n == len(paths) {
			t.Errorf("com o contexto cancelado nem todos deveriam ser processados")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ProcessAll não terminou depois do cancelamento")
	}
}

func TestDocuments(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"R002_leve.jpg", "R001_limpa.png", "R001_forte.JPEG", "notas.txt", ".DS_Store"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	all, err := Documents(dir)
	if err != nil || len(all) != 3 || filepath.Base(all[0]) != "R001_forte.JPEG" {
		t.Errorf("Documents = %v, %v", all, err)
	}
	some, err := Documents(dir, "R002_leve")
	if err != nil || len(some) != 1 {
		t.Errorf("Documents(R002_leve) = %v, %v", some, err)
	}
	if _, err := Documents(dir, "R002_leve", "R099_limpa"); err == nil || !strings.Contains(err.Error(), "R099_limpa") {
		t.Errorf("documento inexistente deveria ser erro, veio %v", err)
	}
}

// TestMatchesPythonPrototype processa R004_limpa (certidão de outra pessoa) com
// o Tesseract de verdade e compara com o resultado gravado pelo protótipo em Python.
// O golden foi gerado com o Tesseract 5.5.3; com outra versão o teste é pulado.
func TestMatchesPythonPrototype(t *testing.T) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		t.Skip("tesseract não está instalado")
	}
	golden, err := ReadResult("testdata/python_R004_limpa.json")
	if err != nil {
		t.Fatal(err)
	}
	engine, err := ocr.NewTesseract(context.Background(), "../../tessdata")
	if err != nil {
		t.Fatal(err)
	}
	if want := golden.Fields[0].Version; engine.Version() != want {
		t.Skipf("golden gerado com o Tesseract %s; instalado %s", want, engine.Version())
	}
	store, err := reference.Open("../../data/base.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	r, err := New(loadConfig(t), engine, store).Process(context.Background(), "../../data/docs/R004_limpa.png")
	if err != nil {
		t.Fatal(err)
	}
	if r.SHA256 != golden.SHA256 || r.Lookup != golden.Lookup || !slices.Equal(r.Alerts, golden.Alerts) {
		t.Errorf("documento difere do Python: %+v", r)
	}
	if math.Abs(r.Sharpness-golden.Sharpness) > 0.1 {
		t.Errorf("nitidez = %v, Python = %v", r.Sharpness, golden.Sharpness)
	}
	for _, want := range golden.Fields {
		got, _ := r.Fields.Get(want.Name)
		if got.Raw != want.Raw || got.Confidence != want.Confidence || got.Decision != want.Decision ||
			got.BBox != want.BBox || deref(got.BaseValue) != deref(want.BaseValue) {
			t.Errorf("%s:\n go     %q %.2f %+v\n python %q %.2f %+v", want.Name,
				got.Raw, got.Confidence, got.Decision, want.Raw, want.Confidence, want.Decision)
		}
	}
}
