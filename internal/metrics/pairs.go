package metrics

import "github.com/Fontoura21/ocr2act-go/internal/jsonenc"

// Pair é uma chave e seu valor.
type Pair[V any] struct {
	Key   string
	Value V
}

// Pairs é um objeto JSON com a ordem das chaves preservada, como os dicts do
// Python que geram o metricas.json original.
type Pairs[V any] []Pair[V]

// Get devolve o valor da chave.
func (p Pairs[V]) Get(key string) (V, bool) {
	for _, kv := range p {
		if kv.Key == key {
			return kv.Value, true
		}
	}
	var zero V
	return zero, false
}

// MarshalJSON codifica os pares como um objeto, na ordem.
func (p Pairs[V]) MarshalJSON() ([]byte, error) {
	buf := []byte{'{'}
	for i, kv := range p {
		if i > 0 {
			buf = append(buf, ',')
		}
		key, err := jsonenc.Marshal(kv.Key)
		if err != nil {
			return nil, err
		}
		value, err := jsonenc.Marshal(kv.Value)
		if err != nil {
			return nil, err
		}
		buf = append(append(append(buf, key...), ':'), value...)
	}
	return append(buf, '}'), nil
}
