// Package reference é a fonte de referência no contrato do Mini-CIC sintético.
// Só responde por chave exata (matrícula ou CPF) e nunca por nome, para
// evitar enumeração de pessoas.
package reference

import (
	"context"
	"fmt"

	"github.com/Fontoura21/ocr2act-go/internal/record"
)

// Entry é o registro como a fonte o devolve, com a marca de dado sintético.
type Entry struct {
	record.Record
	Synthetic int `json:"sintetico"`
}

// Source consulta registros por chave exata. Devolve todos os registros com a
// chave; quem chama decide o que fazer com nenhum ou vários.
type Source interface {
	ByMatricula(ctx context.Context, matricula string) ([]Entry, error)
	ByCPF(ctx context.Context, cpf string) ([]Entry, error)
}

// Como o registro foi localizado, gravado em cada resultado.
const (
	ViaMatricula    = "matricula"
	ViaCPF          = "cpf"
	ViaCPFAmbiguous = "cpf_ambiguo"
	ViaNotFound     = "nao_encontrado"
)

// Find busca pela matrícula e, se ela não resolver, pelo CPF. Nenhum ou
// vários resultados não resolvem: um CPF ambíguo nunca escolhe um registro.
// Chaves que não têm o tamanho certo nem são consultadas.
func Find(ctx context.Context, src Source, matricula, cpf string) (*record.Record, string, error) {
	if isDigits(matricula, 32) {
		entries, err := src.ByMatricula(ctx, matricula)
		if err != nil {
			return nil, "", fmt.Errorf("referência: busca por matrícula: %w", err)
		}
		if len(entries) == 1 {
			return &entries[0].Record, ViaMatricula, nil
		}
	}
	if isDigits(cpf, 11) {
		entries, err := src.ByCPF(ctx, cpf)
		if err != nil {
			return nil, "", fmt.Errorf("referência: busca por CPF: %w", err)
		}
		switch {
		case len(entries) == 1:
			return &entries[0].Record, ViaCPF, nil
		case len(entries) > 1:
			return nil, ViaCPFAmbiguous, nil
		}
	}
	return nil, ViaNotFound, nil
}

func isDigits(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
