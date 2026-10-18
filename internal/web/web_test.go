package web

import (
	"image"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Fontoura21/ocr2act-go/internal/reference"
)

const doc = "R004_limpa" // certidão de outra pessoa: alerta e quatro divergências críticas

// newTestServer monta um diretório de dados com a certidão R004_limpa, o
// resultado gravado pelo protótipo em Python e a verdade de referência.
func newTestServer(t *testing.T, withResult bool) (*httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	copyFile(t, "../../data/docs/"+doc+".png", filepath.Join(dir, "docs", doc+".png"))
	copyFile(t, "../../data/verdade.json", filepath.Join(dir, "verdade.json"))
	if withResult {
		copyFile(t, "../pipeline/testdata/python_"+doc+".json", filepath.Join(dir, "resultados", doc+".json"))
	}
	store, err := reference.Open("../../data/base.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	s, err := New(dir, store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv, dir
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp, string(body)
}

func TestPages(t *testing.T) {
	srv, _ := newTestServer(t, true)
	tests := []struct {
		path string
		want []string
	}{
		{"/", []string{doc, "Documentos processados", "ambiguo"}},
		{"/doc/" + doc, []string{"Possível documento de outra pessoa", "Formulário pré-preenchido", "JULIA FOGAÇA FERREIRA",
			"nome_tokens_confianca_alta", "/img/" + doc + "/recorte/cpf.png", "bloqueado até ação humana"}},
		{"/metricas", []string{"Falso compatível crítico", "calibracao", "Matriz de confusão"}},
		{"/auditoria", []string{"Nenhuma ação de revisão registrada ainda", "íntegra"}},
		{"/docs", []string{"/ref/registros/cpf/{cpf}", "/openapi.yaml"}},
	}
	for _, tt := range tests {
		resp, body := get(t, srv.URL+tt.path)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d", tt.path, resp.StatusCode)
			continue
		}
		for _, w := range tt.want {
			if !strings.Contains(body, w) {
				t.Errorf("GET %s sem %q", tt.path, w)
			}
		}
		if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'self'") {
			t.Errorf("GET %s sem CSP: %q", tt.path, csp)
		}
	}
}

func TestEmptyDataDir(t *testing.T) {
	srv, _ := newTestServer(t, false)
	if _, body := get(t, srv.URL+"/"); !strings.Contains(body, "Nenhum documento processado ainda") {
		t.Error("a página inicial sem resultados deveria orientar a rodar o processamento")
	}
	if _, body := get(t, srv.URL+"/metricas"); !strings.Contains(body, "Ainda não há resultados") {
		t.Error("a página de métricas sem resultados deveria avisar")
	}
}

func TestUnknownOrUnsafeDocumentIs404(t *testing.T) {
	srv, _ := newTestServer(t, true)
	for _, path := range []string{"/doc/R999_limpa", "/doc/..%2F..%2Fetc%2Fpasswd", "/img/R999_limpa/sobreposicao.png",
		"/img/" + doc + "/recorte/nome.jpg", "/img/" + doc + "/recorte/inexistente.png"} {
		if resp, _ := get(t, srv.URL+path); resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, resp.StatusCode)
		}
	}
}

func TestImages(t *testing.T) {
	srv, _ := newTestServer(t, true)
	tests := []struct {
		path string
		size image.Point
	}{
		{"/img/" + doc + "/sobreposicao.png", image.Pt(760, 858)}, // 1240x1400 reduzida mantendo a proporção
		{"/img/" + doc + "/recorte/nome.png", image.Pt(1070, 44)}, // bbox (85, 361, 1155, 405)
	}
	for _, tt := range tests {
		resp, err := http.Get(srv.URL + tt.path)
		if err != nil {
			t.Fatal(err)
		}
		img, format, err := image.Decode(resp.Body)
		_ = resp.Body.Close()
		if err != nil || format != "png" || resp.Header.Get("Content-Type") != "image/png" {
			t.Fatalf("GET %s: %v %s %s", tt.path, err, format, resp.Header.Get("Content-Type"))
		}
		if got := img.Bounds().Size(); got != tt.size {
			t.Errorf("GET %s = %v, want %v", tt.path, got, tt.size)
		}
	}
}

func TestReviewActionIsAudited(t *testing.T) {
	srv, dir := newTestServer(t, true)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	post := func(field string, form url.Values) *http.Response {
		t.Helper()
		resp, err := client.PostForm(srv.URL+"/doc/"+doc+"/campo/"+field, form)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp
	}

	invalid := []struct {
		field string
		form  url.Values
	}{
		{"nome", url.Values{"acao": {"apagar"}}},
		{"nome", url.Values{"acao": {"corrigir"}, "valor": {"   "}}},
		{"inexistente", url.Values{"acao": {"aceitar"}}},
	}
	for _, tt := range invalid {
		if resp := post(tt.field, tt.form); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("POST %s %v = %d, want 400", tt.field, tt.form, resp.StatusCode)
		}
	}

	resp := post("nome", url.Values{"acao": {"corrigir"}, "valor": {"  ANA PAULA MOREIRA "}})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/doc/"+doc+"#nome" {
		t.Fatalf("POST válido = %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	trail, err := os.ReadFile(filepath.Join(dir, "auditoria.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"acao": "corrigir"`, `"valor_corrigido": "ANA PAULA MOREIRA"`, `"valor_lido": "JULIA FOGAÇA FERREIRA"`,
		`"classe_sugerida": "divergencia_real"`, `"hash_anterior": "0000000000000000000000000000000000000000000000000000000000000000"`} {
		if !strings.Contains(string(trail), want) {
			t.Errorf("trilha sem %s:\n%s", want, trail)
		}
	}
	if _, body := get(t, srv.URL+"/doc/"+doc); !strings.Contains(body, "Revisor: <b>corrigir</b>") {
		t.Error("a página do documento deveria mostrar a ação registrada")
	}
	if _, body := get(t, srv.URL+"/auditoria"); !strings.Contains(body, "1 evento(s) íntegros") {
		t.Error("a página de auditoria deveria verificar o evento")
	}
}

func TestCrossOriginPostIsRejected(t *testing.T) {
	srv, dir := newTestServer(t, true)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/doc/"+doc+"/campo/nome", strings.NewReader("acao=aceitar"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("POST de outra origem = %d, want 403", resp.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(dir, "auditoria.jsonl")); !os.IsNotExist(err) {
		t.Error("a ação de outra origem não pode chegar à trilha")
	}
}

func TestReferenceAPIAndHealth(t *testing.T) {
	srv, _ := newTestServer(t, true)
	if resp, body := get(t, srv.URL+"/ref/registros/cpf/34131647591"); resp.StatusCode != http.StatusOK || !strings.Contains(body, "APOLLO") {
		t.Errorf("API de referência = %d %s", resp.StatusCode, body)
	}
	if resp, body := get(t, srv.URL+"/healthz"); resp.StatusCode != http.StatusOK || body != "ok\n" {
		t.Errorf("GET /healthz = %d %q", resp.StatusCode, body)
	}
}
