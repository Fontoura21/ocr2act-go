package reference

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/Fontoura21/ocr2act-go/internal/record"
)

// Registro R001 do conjunto sintético.
const (
	matricula = "83879701552021100036024008667405"
	cpf       = "34131647591"
)

func openDataset(t *testing.T) *Store {
	t.Helper()
	s, err := Open("../../data/base.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestStore(t *testing.T) {
	s := openDataset(t)
	ctx := context.Background()
	byM, err := s.ByMatricula(ctx, matricula)
	if err != nil || len(byM) != 1 {
		t.Fatalf("ByMatricula = %v, %v", byM, err)
	}
	if byM[0].Nome != "APOLLO PORTO SOUZA" || byM[0].CPF != cpf || byM[0].Synthetic != 1 {
		t.Errorf("registro = %+v", byM[0])
	}
	byC, err := s.ByCPF(ctx, cpf)
	if err != nil || len(byC) != 1 || byC[0] != byM[0] {
		t.Errorf("ByCPF = %v, %v", byC, err)
	}
	none, err := s.ByCPF(ctx, "00000000000")
	if err != nil || len(none) != 0 {
		t.Errorf("CPF ausente = %v, %v", none, err)
	}
}

func TestOpenMissingFile(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "nao_existe.sqlite")); err == nil {
		t.Fatal("Open deveria falhar sem criar uma base vazia")
	}
}

// fake é uma fonte em memória para os testes de Find e da API.
type fake struct {
	entries []Entry
	err     error
}

func (f fake) ByMatricula(_ context.Context, m string) ([]Entry, error) {
	return f.filter(func(e Entry) bool { return e.Matricula == m })
}

func (f fake) ByCPF(_ context.Context, c string) ([]Entry, error) {
	return f.filter(func(e Entry) bool { return e.CPF == c })
}

func (f fake) filter(keep func(Entry) bool) ([]Entry, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []Entry
	for _, e := range f.entries {
		if keep(e) {
			out = append(out, e)
		}
	}
	return out, nil
}

func TestFind(t *testing.T) {
	a := Entry{Record: record.Record{Matricula: matricula, CPF: cpf, Nome: "A"}, Synthetic: 1}
	b := Entry{Record: record.Record{Matricula: strings.Repeat("1", 32), CPF: "52998224725", Nome: "B"}, Synthetic: 1}
	c := Entry{Record: record.Record{Matricula: strings.Repeat("2", 32), CPF: "52998224725", Nome: "C"}, Synthetic: 1}
	src := fake{entries: []Entry{a, b, c}}
	tests := []struct {
		name, matricula, cpf, via, nome string
	}{
		{"pela matrícula", matricula, "", ViaMatricula, "A"},
		{"matrícula ausente cai no CPF", strings.Repeat("9", 32), cpf, ViaCPF, "A"},
		{"matrícula com letra não é consultada", "8387970155202110003602400866740B", cpf, ViaCPF, "A"},
		{"CPF ambíguo não escolhe registro", "", "52998224725", ViaCPFAmbiguous, ""},
		{"CPF curto não é consultado", "", cpf[:10], ViaNotFound, ""},
		{"nada encontrado", strings.Repeat("9", 32), "00000000000", ViaNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, via, err := Find(context.Background(), src, tt.matricula, tt.cpf)
			if err != nil || via != tt.via {
				t.Fatalf("Find = %v, %q, %v; want via %q", rec, via, err, tt.via)
			}
			if (rec == nil) != (tt.nome == "") || (rec != nil && rec.Nome != tt.nome) {
				t.Errorf("registro = %+v, want nome %q", rec, tt.nome)
			}
		})
	}
	if _, _, err := Find(context.Background(), fake{err: errors.New("base fora do ar")}, matricula, cpf); err == nil {
		t.Error("erro da fonte deveria ser propagado")
	}
}

func TestFindAmbiguousCPFInSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "base.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE registros (matricula TEXT, cpf TEXT, nome TEXT, data_nascimento TEXT,
		hora_nascimento TEXT, sexo TEXT, municipio TEXT, uf TEXT, mae TEXT, pai TEXT, data_registro TEXT,
		cns TEXT, sintetico INTEGER DEFAULT 1, PRIMARY KEY (matricula));
		INSERT INTO registros (matricula, cpf, nome) VALUES ('1', '52998224725', 'A'), ('2', '52998224725', NULL);`)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	entries, err := s.ByCPF(context.Background(), "52998224725")
	if err != nil || len(entries) != 2 || entries[1].Nome != "" || entries[0].Synthetic != 1 {
		t.Fatalf("ByCPF = %+v, %v", entries, err)
	}
	if _, via, _ := Find(context.Background(), s, "", "52998224725"); via != ViaCPFAmbiguous {
		t.Errorf("via = %q, want %q", via, ViaCPFAmbiguous)
	}
}

func newServer(t *testing.T, src Source) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	Routes(mux, src, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRoutes(t *testing.T) {
	srv := newServer(t, openDataset(t))
	tests := []struct {
		path   string
		status int
		body   string
	}{
		{"/ref/registros/matricula/" + matricula, http.StatusOK, `"nome":"APOLLO PORTO SOUZA"`},
		{"/ref/registros/cpf/" + cpf, http.StatusOK, `"sintetico":1`},
		{"/ref/registros/matricula/123", http.StatusNotFound, `{"detail":"registro não encontrado"}`},
		{"/ref/registros/cpf/00000000000", http.StatusNotFound, `{"detail":"registro não encontrado ou ambíguo"}`},
	}
	for _, tt := range tests {
		resp, err := http.Get(srv.URL + tt.path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != tt.status || !strings.Contains(string(body), tt.body) {
			t.Errorf("GET %s = %d %s, want %d contendo %s", tt.path, resp.StatusCode, body, tt.status, tt.body)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q", ct)
		}
	}
	resp, err := http.Post(srv.URL+"/ref/registros/cpf/"+cpf, "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST = %d, want 405", resp.StatusCode)
	}
}

func TestRoutesFieldOrderMatchesContract(t *testing.T) {
	srv := newServer(t, openDataset(t))
	resp, err := http.Get(srv.URL + "/ref/registros/matricula/" + matricula)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	dec := json.NewDecoder(resp.Body)
	if _, err := dec.Token(); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for dec.More() {
		k, _ := dec.Token()
		keys = append(keys, k.(string))
		var skip json.RawMessage
		_ = dec.Decode(&skip)
	}
	want := "matricula cpf nome data_nascimento hora_nascimento sexo municipio uf mae pai data_registro cns sintetico"
	if got := strings.Join(keys, " "); got != want {
		t.Errorf("campos = %s, want %s", got, want)
	}
}

func TestOpenAPIDocumentsTheRoutes(t *testing.T) {
	srv := newServer(t, fake{})
	resp, err := http.Get(srv.URL + "/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); resp.StatusCode != http.StatusOK || ct != "application/yaml" {
		t.Fatalf("GET /openapi.yaml = %d %s", resp.StatusCode, ct)
	}
	var spec struct {
		OpenAPI string         `yaml:"openapi"`
		Paths   map[string]any `yaml:"paths"`
	}
	if err := yaml.NewDecoder(resp.Body).Decode(&spec); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/ref/registros/matricula/{matricula}", "/ref/registros/cpf/{cpf}", "/healthz"} {
		if _, ok := spec.Paths[p]; !ok {
			t.Errorf("a especificação não documenta %s", p)
		}
	}
	if !strings.HasPrefix(spec.OpenAPI, "3.") {
		t.Errorf("openapi = %q", spec.OpenAPI)
	}
}

func TestRoutesHidesSourceErrors(t *testing.T) {
	srv := newServer(t, fake{err: errors.New("disco cheio em /var/lib/base")})
	resp, err := http.Get(srv.URL + "/ref/registros/cpf/" + cpf)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError || strings.Contains(string(body), "disco") {
		t.Errorf("erro interno = %d %s; não pode vazar detalhes", resp.StatusCode, body)
	}
}

func TestClient(t *testing.T) {
	store := openDataset(t)
	srv := newServer(t, store)
	c, err := NewClient(srv.URL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	got, err := c.ByMatricula(ctx, matricula)
	want, _ := store.ByMatricula(ctx, matricula)
	if err != nil || len(got) != 1 || got[0] != want[0] {
		t.Fatalf("ByMatricula pelo cliente = %+v, %v; want %+v", got, err, want)
	}
	if none, err := c.ByCPF(ctx, "00000000000"); err != nil || none != nil {
		t.Errorf("404 deveria virar lista vazia, veio %v, %v", none, err)
	}
	rec, via, err := Find(ctx, c, matricula, cpf)
	if err != nil || via != ViaMatricula || rec.Nome != "APOLLO PORTO SOUZA" {
		t.Errorf("Find pelo cliente = %+v, %q, %v", rec, via, err)
	}
	failing := newServer(t, fake{err: errors.New("fora do ar")})
	fc, _ := NewClient(failing.URL, nil)
	if _, err := fc.ByCPF(ctx, cpf); err == nil {
		t.Error("500 deveria virar erro")
	}
	for _, bad := range []string{"", "localhost:8080", "ftp://x", "http://"} {
		if _, err := NewClient(bad, nil); err == nil {
			t.Errorf("NewClient(%q) deveria falhar", bad)
		}
	}
}
