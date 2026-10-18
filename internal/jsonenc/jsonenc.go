// Package jsonenc codifica JSON no formato dos arquivos do projeto: sem
// escapar <, > e &, como o json.dumps do Python, e indentado com um espaço.
package jsonenc

import (
	"bytes"
	"encoding/json"
	"io"
)

// Marshal codifica v em JSON compacto, sem escape de HTML.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// Write grava v em w indentado com um espaço, como json.dumps(indent=1).
func Write(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", " ")
	return enc.Encode(v)
}
