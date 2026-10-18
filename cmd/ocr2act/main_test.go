package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(context.Background(), args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestUsage(t *testing.T) {
	if code, _, stderr := runCLI(t); code != 2 || !strings.Contains(stderr, "Uso:") {
		t.Errorf("sem argumentos = %d %q", code, stderr)
	}
	if code, stdout, _ := runCLI(t, "ajuda"); code != 0 || !strings.Contains(stdout, "ocr2act processar") {
		t.Errorf("ajuda = %d %q", code, stdout)
	}
	if code, _, stderr := runCLI(t, "apagar"); code != 2 || !strings.Contains(stderr, "comando desconhecido: apagar") {
		t.Errorf("comando desconhecido = %d %q", code, stderr)
	}
	if code, _, _ := runCLI(t, "processar", "-workers", "0"); code != 2 {
		t.Errorf("-workers 0 = %d, want 2", code)
	}
	if code, _, _ := runCLI(t, "comparar", "só-um-diretório"); code != 2 {
		t.Errorf("comparar com um argumento = %d, want 2", code)
	}
	if code, _, _ := runCLI(t, "metricas", "-h"); code != 0 {
		t.Errorf("metricas -h = %d, want 0", code)
	}
}

// TestEndToEnd roda os comandos sobre duas certidões do conjunto, com o
// Tesseract de verdade, num diretório temporário.
func TestEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		t.Skip("tesseract não está instalado")
	}
	data := t.TempDir()
	for _, f := range []string{"docs/R004_limpa.png", "docs/R002_leve.jpg", "base.sqlite", "verdade.json"} {
		copyFile(t, filepath.Join("../../data", f), filepath.Join(data, f))
	}
	common := []string{"-dados", data, "-config", "../../config/regras.yaml"}

	args := append([]string{"processar", "-tessdata", "../../tessdata", "-workers", "2"}, common...)
	code, stdout, stderr := runCLI(t, args...)
	if code != 0 || !strings.Contains(stdout, "2 documento(s)") || !strings.Contains(stdout, "R004_limpa") || !strings.Contains(stdout, "ALERTA") {
		t.Fatalf("processar = %d\n%s%s", code, stdout, stderr)
	}

	code, stdout, stderr = runCLI(t, append([]string{"metricas"}, common...)...)
	if code != 0 || !strings.Contains(stdout, "== todos (22 campos)") {
		t.Errorf("metricas = %d\n%s%s", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(data, "metricas.json")); err != nil {
		t.Errorf("metricas.json não foi gravado: %v", err)
	}

	if code, stdout, _ := runCLI(t, "auditoria", "-dados", data); code != 0 || !strings.Contains(stdout, "OK: 0 evento(s) íntegros") {
		t.Errorf("auditoria = %d %q", code, stdout)
	}

	// R004_limpa é PNG: o resultado tem de ser idêntico ao do protótipo em Python.
	python := t.TempDir()
	copyFile(t, "../../internal/pipeline/testdata/python_R004_limpa.json", filepath.Join(python, "R004_limpa.json"))
	only := t.TempDir()
	copyFile(t, filepath.Join(data, "resultados", "R004_limpa.json"), filepath.Join(only, "R004_limpa.json"))
	code, stdout, _ = runCLI(t, "comparar", python, only)
	if code != 0 || !strings.Contains(stdout, "decisão idêntica:      11 (100.0%)") {
		t.Errorf("comparar com o protótipo em Python = %d\n%s", code, stdout)
	}

	code, _, stderr = runCLI(t, append(append([]string{"processar", "-tessdata", "../../tessdata"}, common...), "R999_limpa")...)
	if code != 1 || !strings.Contains(stderr, "R999_limpa") {
		t.Errorf("documento inexistente = %d %q", code, stderr)
	}
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
