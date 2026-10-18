package jsonenc

import (
	"bytes"
	"testing"
)

func TestMarshal(t *testing.T) {
	got, err := Marshal(map[string]string{"motivo": "a < b & c > d"})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"motivo":"a < b & c > d"}` {
		t.Errorf("Marshal = %s", got)
	}
}

func TestWrite(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, map[string][]int{"bbox": {1, 2}}); err != nil {
		t.Fatal(err)
	}
	if want := "{\n \"bbox\": [\n  1,\n  2\n ]\n}\n"; buf.String() != want {
		t.Errorf("Write = %q, want %q", buf.String(), want)
	}
}
