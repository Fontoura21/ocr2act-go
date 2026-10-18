package reference

import (
	"context"
	_ "embed"
	"encoding/json"
	"log/slog"
	"net/http"
)

// openAPI documenta o contrato da API de referência.
//
//go:embed openapi.yaml
var openAPI []byte

// Routes registra a API de referência em mux:
//
//	GET /ref/registros/matricula/{matricula}
//	GET /ref/registros/cpf/{cpf}
//	GET /openapi.yaml
//
// As respostas seguem o contrato do protótipo em Python (FastAPI): 200 com o
// registro, 404 com {"detail": ...} quando não há exatamente um registro.
func Routes(mux *http.ServeMux, src Source, log *slog.Logger) {
	mux.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(openAPI)
	})
	mux.HandleFunc("GET /ref/registros/matricula/{matricula}", func(w http.ResponseWriter, r *http.Request) {
		serve(w, r, log, "registro não encontrado", func(ctx context.Context) ([]Entry, error) {
			return src.ByMatricula(ctx, r.PathValue("matricula"))
		})
	})
	mux.HandleFunc("GET /ref/registros/cpf/{cpf}", func(w http.ResponseWriter, r *http.Request) {
		serve(w, r, log, "registro não encontrado ou ambíguo", func(ctx context.Context) ([]Entry, error) {
			return src.ByCPF(ctx, r.PathValue("cpf"))
		})
	})
}

type detail struct {
	Detail string `json:"detail"`
}

func serve(w http.ResponseWriter, r *http.Request, log *slog.Logger, notFound string,
	lookup func(context.Context) ([]Entry, error)) {
	entries, err := lookup(r.Context())
	switch {
	case err != nil:
		log.ErrorContext(r.Context(), "consulta à base de referência falhou", "caminho", r.URL.Path, "erro", err)
		writeJSON(w, http.StatusInternalServerError, detail{"erro ao consultar a base de referência"})
	case len(entries) != 1:
		writeJSON(w, http.StatusNotFound, detail{notFound})
	default:
		writeJSON(w, http.StatusOK, entries[0])
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v) // a resposta já começou; um erro aqui é só conexão fechada
}
