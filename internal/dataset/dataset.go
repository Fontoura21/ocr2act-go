// Package dataset lê a verdade de referência do conjunto sintético
// (verdade.json), escrita pelo gerador junto com as imagens e a base.
package dataset

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Fontoura21/ocr2act-go/internal/record"
)

// Doc é a verdade de uma imagem: o que foi impresso, o que está na base e o
// cenário plantado pelo gerador.
type Doc struct {
	DocID    string `json:"doc_id"`
	RecordID string `json:"registro_id"`
	// Level é o nível de degradação: limpa, leve ou forte.
	Level string `json:"nivel"`
	// Split separa calibração (40% dos registros) de avaliação (60%).
	Split string `json:"split"`
	File  string `json:"arquivo"`
	// Group é o cenário: sem_divergencia, divergencia_real, variacao_grafia
	// ou ambiguo.
	Group         string         `json:"grupo"`
	Type          *string        `json:"tipo"`
	ChangedFields []string       `json:"campos_alterados"`
	InBase        bool           `json:"na_base"`
	Stain         *string        `json:"mancha"`
	Printed       record.Record  `json:"impresso"`
	Base          *record.Record `json:"base"`
}

// Load lê verdade.json e indexa os documentos por doc_id.
func Load(path string) (map[string]Doc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("verdade: %w", err)
	}
	var docs []Doc
	if err := json.Unmarshal(data, &docs); err != nil {
		return nil, fmt.Errorf("verdade: %s: %w", path, err)
	}
	byID := make(map[string]Doc, len(docs))
	for _, d := range docs {
		if _, dup := byID[d.DocID]; dup {
			return nil, fmt.Errorf("verdade: doc_id repetido %s", d.DocID)
		}
		byID[d.DocID] = d
	}
	return byID, nil
}
