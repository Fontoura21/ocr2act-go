// Package pipeline processa uma certidão de ponta a ponta: imagem, OCR por
// região, normalização, busca na base de referência e decisão por campo.
package pipeline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg" // certidões degradadas
	_ "image/png"  // certidões limpas
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Fontoura21/ocr2act-go/internal/config"
	"github.com/Fontoura21/ocr2act-go/internal/ids"
	"github.com/Fontoura21/ocr2act-go/internal/layout"
	"github.com/Fontoura21/ocr2act-go/internal/normalize"
	"github.com/Fontoura21/ocr2act-go/internal/ocr"
	"github.com/Fontoura21/ocr2act-go/internal/reference"
	"github.com/Fontoura21/ocr2act-go/internal/round"
	"github.com/Fontoura21/ocr2act-go/internal/rules"
)

// Pipeline junta o motor de OCR, a fonte de referência e as regras. É seguro
// para uso concorrente se o motor e a fonte também forem.
type Pipeline struct {
	cfg        *config.Config
	engine     ocr.Engine
	refs       reference.Source
	classifier *rules.Classifier
}

// New monta o pipeline.
func New(cfg *config.Config, engine ocr.Engine, refs reference.Source) *Pipeline {
	return &Pipeline{cfg: cfg, engine: engine, refs: refs, classifier: rules.NewClassifier(cfg)}
}

// Process processa o arquivo de imagem em path. O doc_id é o nome do arquivo
// sem extensão.
func (p *Pipeline) Process(ctx context.Context, path string) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("pipeline: %w", err)
	}
	name := filepath.Base(path)
	return p.ProcessImage(ctx, strings.TrimSuffix(name, filepath.Ext(name)), name, data)
}

// ProcessImage processa uma imagem PNG ou JPEG já carregada.
func (p *Pipeline) ProcessImage(ctx context.Context, docID, fileName string, data []byte) (*Result, error) {
	start := time.Now()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("pipeline: %s: %w", fileName, err)
	}
	gray := ocr.Grayscale(img)
	sharpness := ocr.Sharpness(gray)
	qualityOK := sharpness >= p.cfg.Quality.MinSharpness

	bounds := img.Bounds()
	var fields Fields
	for _, f := range layout.Fields() {
		box := f.Region(bounds.Size(), layout.DefaultInset)
		region, ok := gray.SubImage(box.Add(bounds.Min)).(*image.Gray)
		if !ok || region.Bounds().Empty() {
			return nil, fmt.Errorf("pipeline: %s: região de %s fora da imagem", docID, f.Name)
		}
		reading, err := p.engine.Read(ctx, region)
		if err != nil {
			return nil, fmt.Errorf("pipeline: %s, campo %s: %w", docID, f.Name, err)
		}
		value, valid, transforms := normalize.Apply(f.Kind, reading.Text)
		var normalized *string
		if valid {
			normalized = &value
		}
		fields = append(fields, Field{
			Name: f.Name, Label: f.Label, Critical: f.Critical, Kind: f.Kind,
			BBox: [4]int{box.Min.X, box.Min.Y, box.Max.X, box.Max.Y},
			Raw:  reading.Text, Normalized: normalized, Transforms: transforms,
			Confidence: reading.Confidence, Words: reading.Words,
			Engine: reading.Engine, Version: reading.Version,
		})
	}

	matricula, _ := fields.Get("matricula")
	cpf, _ := fields.Get("cpf")
	rec, via, err := reference.Find(ctx, p.refs, deref(matricula.Normalized), deref(cpf.Normalized))
	if err != nil {
		return nil, fmt.Errorf("pipeline: %s: %w", docID, err)
	}

	var divergent []string
	for i := range fields {
		f := &fields[i]
		spec, _ := layout.ByName(f.Name)
		if rec != nil {
			base := rec.Display(f.Name)
			f.BaseValue = &base
		}
		f.Decision = p.classifier.Classify(spec, rules.Reading{
			Raw: f.Raw, Normalized: deref(f.Normalized), Valid: f.Normalized != nil, Confidence: f.Confidence,
		}, rec)
		if !qualityOK && f.Decision.Class != rules.Review {
			f.Decision = rules.Decision{Class: rules.Review, Rule: "qualidade_baixa",
				Reason: fmt.Sprintf("captura abaixo do limiar de nitidez (%.0f)", sharpness)}
		}
		if f.Critical && f.Decision.Class == rules.RealDivergence {
			divergent = append(divergent, f.Name)
		}
	}

	alerts := []string{}
	if len(divergent) >= p.cfg.OtherPersonMinFields {
		alerts = append(alerts, fmt.Sprintf("Possível documento de outra pessoa: %d campos críticos divergem (%s). "+
			"Documento inteiro vai para revisão.", len(divergent), strings.Join(divergent, ", ")))
	}
	var masked *string
	if rec == nil {
		alerts = append(alerts, fmt.Sprintf("Registro não localizado na base (busca: %s).", via))
	} else {
		m := ids.MaskCPF(rec.CPF)
		masked = &m
	}

	sum := sha256.Sum256(data)
	return &Result{
		DocID: docID, File: fileName, SHA256: hex.EncodeToString(sum[:]),
		Sharpness: round.To(sharpness, 1), QualityOK: qualityOK, DocType: "nascimento",
		Lookup: via, MaskedCPF: masked, RulesVersion: p.cfg.Version,
		Fields: fields, Alerts: alerts, Seconds: round.To(time.Since(start).Seconds(), 2),
	}, nil
}

// Outcome é o desfecho de um arquivo processado em lote.
type Outcome struct {
	Path   string
	Result *Result
	Err    error
}

// ProcessAll processa os arquivos com até workers documentos ao mesmo tempo e
// entrega cada desfecho assim que fica pronto, em ordem de conclusão. O canal
// é fechado quando todos terminam; com o contexto cancelado, os workers
// param no próximo ponto de espera. Quem chama deve ler até o fechamento.
func (p *Pipeline) ProcessAll(ctx context.Context, paths []string, workers int) <-chan Outcome {
	jobs := make(chan string)
	out := make(chan Outcome)
	var wg sync.WaitGroup
	for range max(workers, 1) {
		wg.Go(func() {
			for path := range jobs {
				r, err := p.Process(ctx, path)
				select {
				case out <- Outcome{Path: path, Result: r, Err: err}:
				case <-ctx.Done():
					return
				}
			}
		})
	}
	go func() {
		defer close(jobs)
		for _, path := range paths {
			select {
			case jobs <- path:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

// Documents lista as imagens PNG e JPEG de dir em ordem de nome. Com docIDs,
// só esses documentos; pedir um que não existe é erro.
func Documents(dir string, docIDs ...string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	wanted := make(map[string]bool, len(docIDs))
	for _, id := range docIDs {
		wanted[id] = true
	}
	var paths []string
	for _, e := range entries {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if e.IsDir() || (ext != ".png" && ext != ".jpg" && ext != ".jpeg") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		if len(docIDs) > 0 && !wanted[id] {
			continue
		}
		delete(wanted, id)
		paths = append(paths, filepath.Join(dir, e.Name()))
	}
	if len(wanted) > 0 {
		missing := make([]string, 0, len(wanted))
		for id := range wanted {
			missing = append(missing, id)
		}
		slices.Sort(missing)
		return nil, fmt.Errorf("pipeline: documento(s) não encontrado(s) em %s: %s", dir, strings.Join(missing, ", "))
	}
	return paths, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
