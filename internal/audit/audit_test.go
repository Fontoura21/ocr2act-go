package audit

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// testdata/python.jsonl foi gravado pelo protótipo em Python (ocr2act.audit) com os
// dois eventos abaixo. O primeiro tem aspas, tabulação, barra invertida e
// acentos, que exercitam o escape.
var pythonEvents = []Event{
	{
		DocID: "R003_limpa", DocSHA256: "a6d38de6", Field: "nome", Action: "corrigir",
		ReadValue: "ANA \"PAULA\"\tCOSTA\\ é ç", CorrectedValue: ptr("ANA PAULA MOREIRA"),
		SuggestedClass: "divergencia_real", Rule: "nome_tokens_confianca_alta",
		Engine: "tesseract", EngineVersion: "5.5.3", RulesVersion: "0.1.0",
	},
	{
		DocID: "R003_limpa", DocSHA256: "a6d38de6", Field: "cpf", Action: "aceitar",
		ReadValue: "166.978.480-06", SuggestedClass: "compativel", Rule: "id_igual",
		Engine: "tesseract", EngineVersion: "5.5.3", RulesVersion: "0.1.0",
	},
}

var pythonTimes = []time.Time{
	time.Date(2026, 10, 18, 15, 0, 0, 0, time.UTC),
	time.Date(2026, 10, 18, 12, 1, 0, 0, time.FixedZone("BRT", -3*3600)), // 15:01 UTC
}

func TestVerifiesTrailWrittenByPython(t *testing.T) {
	v, err := New("testdata/python.jsonl").Verify()
	if err != nil {
		t.Fatal(err)
	}
	if !v.OK || v.Events != 2 || v.Message != "2 evento(s) íntegros" {
		t.Errorf("Verify = %+v", v)
	}
}

func TestWritesTheSameBytesAsPython(t *testing.T) {
	log := New(filepath.Join(t.TempDir(), "auditoria.jsonl"))
	for i, e := range pythonEvents {
		log.now = func() time.Time { return pythonTimes[i] }
		got, err := log.Append(e)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && got.Hash != "e4bdd0f341ef4e3b67e81a9c6300941356b994855bb02002050e63270e1c8e7c" {
			t.Errorf("hash do primeiro evento = %s", got.Hash)
		}
	}
	got, err := os.ReadFile(log.Path())
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/python.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("trilha difere da gravada pelo Python:\n go:     %s\n python: %s", got, want)
	}
}

func TestDetectsTampering(t *testing.T) {
	original, err := os.ReadFile("testdata/python.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.SplitAfter(string(original), "\n")
	tests := []struct {
		name, content, want string
	}{
		{"valor corrigido alterado", strings.Replace(string(original), "MOREIRA", "MOREIRAS", 1), "evento 1: conteúdo alterado"},
		{"primeiro evento removido", lines[1], "evento 1: encadeamento quebrado"},
		{"eventos invertidos", lines[1] + lines[0], "evento 1: encadeamento quebrado"},
		{"linha que não é JSON", string(original) + "{quebrado\n", "evento 3: JSON inválido"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "auditoria.jsonl")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}
			v, err := New(path).Verify()
			if err != nil || v.OK || v.Message != tt.want {
				t.Errorf("Verify = %+v, %v; want falha %q", v, err, tt.want)
			}
		})
	}
}

func TestEmptyTrail(t *testing.T) {
	log := New(filepath.Join(t.TempDir(), "auditoria.jsonl"))
	events, err := log.Events()
	if err != nil || len(events) != 0 {
		t.Errorf("Events = %v, %v", events, err)
	}
	if v, err := log.Verify(); err != nil || !v.OK || v.Events != 0 {
		t.Errorf("Verify = %+v, %v", v, err)
	}
}

func TestConcurrentAppendsKeepTheChain(t *testing.T) {
	log := New(filepath.Join(t.TempDir(), "auditoria.jsonl"))
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if _, err := log.Append(pythonEvents[1]); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	v, err := log.Verify()
	if err != nil || !v.OK || v.Events != 20 {
		t.Errorf("Verify = %+v, %v", v, err)
	}
	events, _ := log.Events()
	if events[0].PrevHash != Genesis || events[19].PrevHash != events[18].Hash {
		t.Error("a cadeia não liga cada evento ao anterior")
	}
}

func TestAppendReplacesInvalidUTF8(t *testing.T) {
	log := New(filepath.Join(t.TempDir(), "auditoria.jsonl"))
	e := pythonEvents[1]
	e.CorrectedValue = ptr("ANA\xffPAULA")
	got, err := log.Append(e)
	if err != nil {
		t.Fatal(err)
	}
	if *got.CorrectedValue != "ANA\uFFFDPAULA" {
		t.Errorf("valor corrigido = %q", *got.CorrectedValue)
	}
	if v, _ := log.Verify(); !v.OK {
		t.Errorf("trilha com o valor saneado deveria verificar: %+v", v)
	}
}

func TestQuote(t *testing.T) {
	// Saídas conferidas com json.dumps(s, ensure_ascii=False).
	tests := map[string]string{
		"simples":        `"simples"`,
		"aspas \" e \\":  `"aspas \" e \\"`,
		"\b\f\n\r\t":     `"\b\f\n\r\t"`,
		"\x00\x1f\x7f":   "\"\\u0000\\u001f\x7f\"",
		"ação <b> & ç":   `"ação <b> & ç"`,
		"linha\u2028fim": "\"linha\u2028fim\"",
	}
	for in, want := range tests {
		if got := quote(in); got != want {
			t.Errorf("quote(%q) = %s, want %s", in, got, want)
		}
	}
}

func ptr(s string) *string { return &s }
