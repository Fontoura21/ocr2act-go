// Package server sobe os servidores HTTP do projeto com tempos limite,
// log de requisições e desligamento gracioso.
package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"
)

// Addr devolve o endereço de escuta: o da flag quando informado; senão
// :$PORT, a variável que o Cloud Run define; senão fallback.
func Addr(flag, fallback string) string {
	if flag != "" {
		return flag
	}
	if port := os.Getenv("PORT"); port != "" {
		return ":" + port
	}
	return fallback
}

// Run serve h em addr até ctx ser cancelado. Então para de aceitar conexões e
// espera as requisições em andamento por até 10 segundos.
func Run(ctx context.Context, addr string, h http.Handler, log *slog.Logger) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return Serve(ctx, ln, h, log)
}

// Serve é o Run com um listener já aberto, útil quando a porta é escolhida
// pelo sistema.
func Serve(ctx context.Context, ln net.Listener, h http.Handler, log *slog.Logger) error {
	srv := &http.Server{
		Handler:           Logging(log, h),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	log.Info("servidor no ar", "endereco", ln.Addr().String())
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("desligando o servidor")
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil {
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// CloudLogger grava logs em JSON com as chaves que o Cloud Logging reconhece:
// "severity" no lugar de "level" (com WARNING em vez de WARN) e "message" no
// lugar de "msg".
func CloudLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) > 0 {
				return a
			}
			switch a.Key {
			case slog.LevelKey:
				level, _ := a.Value.Any().(slog.Level)
				severity := level.String()
				if level == slog.LevelWarn {
					severity = "WARNING"
				}
				return slog.String("severity", severity)
			case slog.MessageKey:
				a.Key = "message"
			}
			return a
		},
	}))
}

// Health responde 200 "ok", para verificação de saúde (GET /healthz).
func Health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok\n"))
}

// Logging registra método, caminho, status e duração de cada requisição.
func Logging(log *slog.Logger, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		h.ServeHTTP(rec, r)
		log.LogAttrs(r.Context(), slog.LevelInfo, "requisição",
			slog.String("metodo", r.Method), slog.String("caminho", r.URL.Path), slog.Int("status", rec.status),
			slog.Float64("duracao_ms", float64(time.Since(start).Microseconds())/1000))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Unwrap permite ao http.ResponseController alcançar o writer original.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
