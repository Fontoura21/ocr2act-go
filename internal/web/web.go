// Package web é a interface de revisão humana: lista os documentos
// processados, mostra cada campo com o recorte, o valor lido, o da base e o
// motivo da decisão, e registra as ações do revisor na trilha de auditoria.
// Nenhum campo crítico é confirmado sem ação humana.
package web

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"image/color"
	"io/fs"
	"log/slog"
	"net/http"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Fontoura21/ocr2act-go/internal/audit"
	"github.com/Fontoura21/ocr2act-go/internal/dataset"
	"github.com/Fontoura21/ocr2act-go/internal/metrics"
	"github.com/Fontoura21/ocr2act-go/internal/pipeline"
	"github.com/Fontoura21/ocr2act-go/internal/reference"
	"github.com/Fontoura21/ocr2act-go/internal/rules"
	"github.com/Fontoura21/ocr2act-go/internal/server"
)

//go:embed templates/*.html
var templateFS embed.FS

// docID aceita só nomes simples, sem ponto nem barra, para que o id da URL
// nunca saia do diretório de resultados.
var docID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// classView é uma classe com rótulo e cor para a interface.
type classView struct {
	Class rules.Class
	Label string
	Color template.CSS
	rgba  color.RGBA
}

var classes = []classView{
	{rules.Compatible, rules.Compatible.Label(), "#1a7f37", color.RGBA{0x1a, 0x7f, 0x37, 0xff}},
	{rules.OCRError, rules.OCRError.Label(), "#b7791f", color.RGBA{0xb7, 0x79, 0x1f, 0xff}},
	{rules.RealDivergence, rules.RealDivergence.Label(), "#c62828", color.RGBA{0xc6, 0x28, 0x28, 0xff}},
	{rules.Review, rules.Review.Label(), "#3949ab", color.RGBA{0x39, 0x49, 0xab, 0xff}},
}

func classOf(c rules.Class) classView {
	if i := slices.IndexFunc(classes, func(v classView) bool { return v.Class == c }); i >= 0 {
		return classes[i]
	}
	return classView{Class: c, Label: string(c), Color: "#5f6b7a", rgba: color.RGBA{0x5f, 0x6b, 0x7a, 0xff}}
}

// Server serve a interface de revisão e a API de referência.
type Server struct {
	results string
	docs    string
	truth   map[string]dataset.Doc
	audit   *audit.Log
	refs    reference.Source
	log     *slog.Logger
	pages   map[string]*template.Template
}

// New prepara a interface sobre o diretório do conjunto (docs/, resultados/,
// verdade.json e auditoria.jsonl). A verdade é opcional: sem ela a interface
// funciona, só não mostra o cenário de cada documento.
func New(dataDir string, refs reference.Source, log *slog.Logger) (*Server, error) {
	truth, err := dataset.Load(filepath.Join(dataDir, "verdade.json"))
	if errors.Is(err, fs.ErrNotExist) {
		truth = map[string]dataset.Doc{}
	} else if err != nil {
		return nil, err
	}
	pages := map[string]*template.Template{}
	funcs := template.FuncMap{"join": strings.Join}
	for _, name := range []string{"index", "doc", "metricas", "auditoria", "docs"} {
		t, err := template.New(name).Funcs(funcs).ParseFS(templateFS, "templates/base.html", "templates/"+name+".html")
		if err != nil {
			return nil, fmt.Errorf("web: template %s: %w", name, err)
		}
		pages[name] = t
	}
	return &Server{
		results: filepath.Join(dataDir, "resultados"),
		docs:    filepath.Join(dataDir, "docs"),
		truth:   truth,
		audit:   audit.New(filepath.Join(dataDir, "auditoria.jsonl")),
		refs:    refs,
		log:     log,
		pages:   pages,
	}, nil
}

// Handler devolve as rotas da interface e da API de referência, protegidas
// contra requisições de outra origem (CSRF) nos métodos que alteram estado.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /doc/{docID}", s.document)
	mux.HandleFunc("POST /doc/{docID}/campo/{field}", s.act)
	mux.HandleFunc("GET /img/{docID}/sobreposicao.png", s.overlay)
	mux.HandleFunc("GET /img/{docID}/recorte/{file}", s.crop)
	mux.HandleFunc("GET /metricas", s.metrics)
	mux.HandleFunc("GET /auditoria", s.auditTrail)
	mux.HandleFunc("GET /docs", s.apiDocs)
	mux.HandleFunc("GET /healthz", server.Health)
	reference.Routes(mux, s.refs, s.log)
	return http.NewCrossOriginProtection().Handler(mux)
}

type docRow struct {
	Result *pipeline.Result
	Counts map[rules.Class]int
	Truth  *dataset.Doc
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	results, err := s.readResults()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	rows := make([]docRow, len(results))
	for i, res := range results {
		counts := map[rules.Class]int{}
		for _, f := range res.Fields {
			counts[f.Decision.Class]++
		}
		rows[i] = docRow{Result: res, Counts: counts, Truth: s.truthOf(res.DocID)}
	}
	s.render(w, r, "index", map[string]any{"Docs": rows, "Classes": classes})
}

type fieldView struct {
	pipeline.Field
	Class   classView
	Action  *audit.Event
	Printed string
	Changed bool
}

func (s *Server) document(w http.ResponseWriter, r *http.Request) {
	res, ok := s.result(w, r)
	if !ok {
		return
	}
	events, err := s.audit.Events()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	last := map[string]audit.Event{}
	for _, e := range events {
		if e.DocID == res.DocID {
			last[e.Field] = e
		}
	}
	truth := s.truthOf(res.DocID)
	fields := make([]fieldView, len(res.Fields))
	for i, f := range res.Fields {
		v := fieldView{Field: f, Class: classOf(f.Decision.Class)}
		if e, ok := last[f.Name]; ok {
			v.Action = &e
		}
		if truth != nil {
			v.Printed = truth.Printed.Display(f.Name)
			v.Changed = slices.Contains(truth.ChangedFields, f.Name)
		}
		fields[i] = v
	}
	s.render(w, r, "doc", map[string]any{"R": res, "Fields": fields, "Classes": classes, "Truth": truth})
}

// act registra a ação do revisor sobre um campo.
func (s *Server) act(w http.ResponseWriter, r *http.Request) {
	res, ok := s.result(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "formulário inválido", http.StatusBadRequest)
		return
	}
	name, action := r.PathValue("field"), r.PostForm.Get("acao")
	value := strings.TrimSpace(r.PostForm.Get("valor"))
	f, ok := res.Fields.Get(name)
	if !ok || !slices.Contains([]string{"aceitar", "corrigir", "rejeitar"}, action) {
		http.Error(w, "ação inválida", http.StatusBadRequest)
		return
	}
	if action == "corrigir" && value == "" {
		http.Error(w, "informe o valor corrigido", http.StatusBadRequest)
		return
	}
	var corrected *string
	if value != "" {
		corrected = &value
	}
	_, err := s.audit.Append(audit.Event{
		DocID: res.DocID, DocSHA256: res.SHA256, Field: name, Action: action,
		ReadValue: f.Raw, CorrectedValue: corrected,
		SuggestedClass: string(f.Decision.Class), Rule: f.Decision.Rule,
		Engine: f.Engine, EngineVersion: f.Version, RulesVersion: res.RulesVersion,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/doc/"+res.DocID+"#"+name, http.StatusSeeOther)
}

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{"Classes": classes}
	results, err := s.readResults()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if len(results) > 0 && len(s.truth) > 0 {
		report, err := metrics.Compute(results, s.truth)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		data["Report"] = report
	}
	s.render(w, r, "metricas", data)
}

func (s *Server) auditTrail(w http.ResponseWriter, r *http.Request) {
	events, err := s.audit.Events()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v, err := s.audit.Verify()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	slices.Reverse(events)
	s.render(w, r, "auditoria", map[string]any{"Events": events, "Verification": v})
}

func (s *Server) apiDocs(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "docs", nil)
}

// result carrega o resultado do documento da URL, respondendo 404 se ele não
// existe. ok falso significa que a resposta já foi escrita.
func (s *Server) result(w http.ResponseWriter, r *http.Request) (*pipeline.Result, bool) {
	id := r.PathValue("docID")
	if !docID.MatchString(id) {
		http.Error(w, "documento não processado", http.StatusNotFound)
		return nil, false
	}
	res, err := pipeline.ReadResult(filepath.Join(s.results, id+".json"))
	if errors.Is(err, fs.ErrNotExist) {
		http.Error(w, "documento não processado", http.StatusNotFound)
		return nil, false
	}
	if err != nil {
		s.fail(w, r, err)
		return nil, false
	}
	return res, true
}

func (s *Server) readResults() ([]*pipeline.Result, error) {
	results, err := pipeline.ReadResults(s.results)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return results, err
}

func (s *Server) truthOf(id string) *dataset.Doc {
	if d, ok := s.truth[id]; ok {
		return &d
	}
	return nil
}

// render executa a página num buffer antes de responder, para que um erro no
// template não deixe uma página pela metade.
func (s *Server) render(w http.ResponseWriter, r *http.Request, page string, data any) {
	var buf bytes.Buffer
	if err := s.pages[page].ExecuteTemplate(&buf, "base", data); err != nil {
		s.fail(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy",
		"default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self'; form-action 'self'; frame-ancestors 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	_, _ = buf.WriteTo(w)
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	s.log.ErrorContext(r.Context(), "falha na interface de revisão", "caminho", r.URL.Path, "erro", err)
	http.Error(w, "erro interno", http.StatusInternalServerError)
}
