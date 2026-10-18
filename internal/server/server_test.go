package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAddr(t *testing.T) {
	t.Setenv("PORT", "")
	if got := Addr("", ":8000"); got != ":8000" {
		t.Errorf("sem flag nem PORT = %q", got)
	}
	t.Setenv("PORT", "9090")
	if got := Addr("", ":8000"); got != ":9090" {
		t.Errorf("com PORT = %q", got)
	}
	if got := Addr("127.0.0.1:7000", ":8000"); got != "127.0.0.1:7000" {
		t.Errorf("a flag deve ter precedência sobre PORT, veio %q", got)
	}
}

func TestCloudLogger(t *testing.T) {
	var buf bytes.Buffer
	log := CloudLogger(&buf)
	log.Warn("base lenta", "ms", 1200)
	log.Info("ok")
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var first, second map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatal(err)
	}
	if first["severity"] != "WARNING" || first["message"] != "base lenta" || first["ms"] != 1200.0 {
		t.Errorf("linha de aviso = %v", first)
	}
	if second["severity"] != "INFO" || first["level"] != nil || first["msg"] != nil {
		t.Errorf("chaves fora do formato do Cloud Logging: %v %v", first, second)
	}
}

func TestServeAndGracefulShutdown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logs, nil))
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", Health)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, mux, log) }()

	resp, err := http.Get("http://" + ln.Addr().String() + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "ok\n" {
		t.Errorf("GET /healthz = %d %q", resp.StatusCode, body)
	}
	resp, err = http.Get("http://" + ln.Addr().String() + "/inexistente")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve = %v, want nil depois do desligamento", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("o servidor não desligou")
	}
	for _, want := range []string{"caminho=/healthz", "status=200", "status=404", "desligando"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log sem %q:\n%s", want, logs.String())
		}
	}
}
