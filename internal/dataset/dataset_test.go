package dataset

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDataset(t *testing.T) {
	docs, err := Load("../../data/verdade.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 90 {
		t.Fatalf("len = %d, want 90 (30 registros x 3 níveis)", len(docs))
	}
	d := docs["R001_limpa"]
	if d.Level != "limpa" || d.Group != "variacao_grafia" || d.Base == nil || d.Printed.Nome != "APOLLO PORTO SOUSA" {
		t.Errorf("R001_limpa = %+v", d)
	}
	missing := 0
	for _, d := range docs {
		if d.Base == nil {
			missing++
			if d.InBase {
				t.Errorf("%s: sem base mas na_base = true", d.DocID)
			}
		}
	}
	if missing == 0 {
		t.Error("o conjunto deveria ter registros ausentes da base")
	}
}

func TestLoadRejectsDuplicates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "verdade.json")
	if err := os.WriteFile(path, []byte(`[{"doc_id":"R001_limpa"},{"doc_id":"R001_limpa"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("doc_id repetido deveria ser erro")
	}
}
