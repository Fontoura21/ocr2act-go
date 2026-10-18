package layout

import (
	"image"
	"testing"
)

func TestFields(t *testing.T) {
	fs := Fields()
	if len(fs) != 11 {
		t.Fatalf("len(Fields()) = %d, want 11", len(fs))
	}
	fs[0].Name = "alterado"
	if Fields()[0].Name != "nome" {
		t.Fatal("Fields() deve devolver uma cópia")
	}
	critical := 0
	for _, f := range Fields() {
		if f.Critical {
			critical++
		}
	}
	if critical != 6 {
		t.Errorf("campos críticos = %d, want 6", critical)
	}
	if f, ok := ByName("cpf"); !ok || f.Kind != KindIdentifier {
		t.Errorf("ByName(cpf) = %+v, %v", f, ok)
	}
	if _, ok := ByName("inexistente"); ok {
		t.Error("ByName(inexistente) deveria falhar")
	}
}

func TestRegion(t *testing.T) {
	nome, _ := ByName("nome")
	if got, want := nome.ValueBox(), image.Rect(80, 356, 1160, 410); got != want {
		t.Errorf("ValueBox = %v, want %v", got, want)
	}
	// Na página de referência a região é a caixa recuada 5 px (bbox do protótipo em Python).
	if got, want := nome.Region(image.Pt(PageWidth, PageHeight), DefaultInset), image.Rect(85, 361, 1155, 405); got != want {
		t.Errorf("Region na escala 1 = %v, want %v", got, want)
	}
	// Na metade do tamanho, 85*0.5 = 42.5 arredonda para 42 (meio para par).
	if got, want := nome.Region(image.Pt(PageWidth/2, PageHeight/2), DefaultInset), image.Rect(42, 180, 578, 202); got != want {
		t.Errorf("Region na escala 0.5 = %v, want %v", got, want)
	}
}
