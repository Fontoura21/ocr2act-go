package audit

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// member é um par chave-valor de um objeto JSON com ordem definida.
type member struct {
	key   string
	value any
}

// encode reproduz o json.dumps do Python com ensure_ascii=False e os
// separadores padrão (", " e ": "). Com sortKeys, as chaves dos objetos saem
// em ordem, como com sort_keys=True. Aceita os tipos que o encoding/json
// produz com UseNumber, além de []member para objetos em ordem fixa.
func encode(v any, sortKeys bool) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(x)
	case string:
		return quote(x)
	case json.Number:
		return x.String()
	case []any:
		parts := make([]string, len(x))
		for i, item := range x {
			parts[i] = encode(item, sortKeys)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		members := make([]member, 0, len(x))
		for k, val := range x {
			members = append(members, member{k, val})
		}
		return encodeObject(members, true)
	case []member:
		return encodeObject(x, sortKeys)
	}
	panic(fmt.Sprintf("audit: tipo sem codificação: %T", v))
}

func encodeObject(members []member, sortKeys bool) string {
	if sortKeys {
		members = slices.Clone(members)
		// Ordem de bytes do UTF-8 é a ordem de code points do sort do Python.
		slices.SortFunc(members, func(a, b member) int { return strings.Compare(a.key, b.key) })
	}
	parts := make([]string, len(members))
	for i, m := range members {
		parts[i] = quote(m.key) + ": " + encode(m.value, sortKeys)
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// quote escapa como o Python com ensure_ascii=False: aspas, barra invertida
// e caracteres de controle; o resto do Unicode sai como está.
func quote(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
