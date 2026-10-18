package reference

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"

	_ "modernc.org/sqlite" // driver SQLite em Go puro, sem cgo
)

// Store é a base de referência em SQLite, aberta só para leitura.
type Store struct {
	db *sql.DB
}

const query = `SELECT matricula, cpf, nome, data_nascimento, hora_nascimento, sexo,
	municipio, uf, mae, pai, data_registro, cns, sintetico FROM registros WHERE `

// Open abre a base em modo somente leitura. O arquivo precisa existir: o
// SQLite criaria uma base vazia em silêncio.
func Open(path string) (*Store, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("referência: %w", err)
	}
	dsn := (&url.URL{Scheme: "file", OmitHost: true, Path: path, RawQuery: "mode=ro"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("referência: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, errors.Join(fmt.Errorf("referência: %w", err), db.Close())
	}
	return &Store{db: db}, nil
}

// Close fecha a base.
func (s *Store) Close() error { return s.db.Close() }

// ByMatricula devolve os registros com a matrícula dada.
func (s *Store) ByMatricula(ctx context.Context, matricula string) ([]Entry, error) {
	return s.find(ctx, query+"matricula = ?", matricula)
}

// ByCPF devolve os registros com o CPF dado.
func (s *Store) ByCPF(ctx context.Context, cpf string) ([]Entry, error) {
	return s.find(ctx, query+"cpf = ?", cpf)
}

func (s *Store) find(ctx context.Context, q, key string) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, q, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// scanEntry lê uma linha na ordem das colunas de query. Colunas nulas viram
// texto vazio.
func scanEntry(rows *sql.Rows) (Entry, error) {
	var e Entry
	r := &e.Record
	fields := []*string{&r.Matricula, &r.CPF, &r.Nome, &r.DataNascimento, &r.HoraNascimento, &r.Sexo,
		&r.Municipio, &r.UF, &r.Mae, &r.Pai, &r.DataRegistro, &r.CNS}
	raw := make([]sql.NullString, len(fields))
	var synthetic sql.NullInt64
	dest := make([]any, 0, len(fields)+1)
	for i := range raw {
		dest = append(dest, &raw[i])
	}
	if err := rows.Scan(append(dest, &synthetic)...); err != nil {
		return Entry{}, err
	}
	for i, f := range fields {
		*f = raw[i].String
	}
	e.Synthetic = int(synthetic.Int64)
	return e, nil
}
