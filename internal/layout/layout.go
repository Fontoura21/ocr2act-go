// Package layout descreve o template de regiões da certidão de nascimento
// sintética (schema v1). As coordenadas são da página de referência; Region
// as escala para o tamanho real da imagem.
package layout

import (
	"image"
	"math"
	"slices"
)

// Dimensões da página de referência, em pixels.
const (
	PageWidth   = 1240
	PageHeight  = 1400
	LabelHeight = 26
	// DefaultInset descarta a borda da caixa para que o traço do retângulo
	// não entre no recorte lido pelo OCR.
	DefaultInset = 5
)

// Kind é o tipo do campo, que decide a normalização e a regra de comparação.
type Kind string

// Tipos de campo.
const (
	KindName       Kind = "nome"
	KindIdentifier Kind = "identificador"
	KindDate       Kind = "data"
	KindExact      Kind = "exato"
)

// Field é um campo da certidão: rótulo impresso e caixa do valor.
type Field struct {
	Name       string
	Label      string
	X, Y, W, H int
	Kind       Kind
	// Critical marca os campos que nunca são confirmados sem ação humana.
	Critical bool
}

var fields = []Field{
	{Name: "nome", Label: "NOME", X: 80, Y: 330, W: 1080, H: 54, Kind: KindName, Critical: true},
	{Name: "matricula", Label: "MATRÍCULA", X: 80, Y: 430, W: 1080, H: 54, Kind: KindIdentifier, Critical: true},
	{Name: "cpf", Label: "CPF", X: 80, Y: 530, W: 440, H: 54, Kind: KindIdentifier, Critical: true},
	{Name: "data_nascimento", Label: "DATA DE NASCIMENTO", X: 560, Y: 530, W: 340, H: 54, Kind: KindDate, Critical: true},
	{Name: "hora_nascimento", Label: "HORA", X: 940, Y: 530, W: 220, H: 54, Kind: KindExact},
	{Name: "sexo", Label: "SEXO", X: 80, Y: 630, W: 340, H: 54, Kind: KindExact},
	{Name: "municipio_uf", Label: "MUNICÍPIO DE NASCIMENTO / UF", X: 460, Y: 630, W: 700, H: 54, Kind: KindExact},
	{Name: "mae", Label: "FILIAÇÃO: MÃE", X: 80, Y: 730, W: 1080, H: 54, Kind: KindName, Critical: true},
	{Name: "pai", Label: "FILIAÇÃO: PAI", X: 80, Y: 830, W: 1080, H: 54, Kind: KindName, Critical: true},
	{Name: "data_registro", Label: "DATA DO REGISTRO", X: 80, Y: 930, W: 400, H: 54, Kind: KindDate},
	{Name: "cns", Label: "CNS DA SERVENTIA", X: 520, Y: 930, W: 300, H: 54, Kind: KindExact},
}

// Fields devolve os campos na ordem do template. A ordem é a mesma dos
// resultados e das métricas.
func Fields() []Field {
	return slices.Clone(fields)
}

// ByName devolve o campo com o nome dado.
func ByName(name string) (Field, bool) {
	i := slices.IndexFunc(fields, func(f Field) bool { return f.Name == name })
	if i < 0 {
		return Field{}, false
	}
	return fields[i], true
}

// ValueBox é a caixa do valor na página de referência, logo abaixo do rótulo.
func (f Field) ValueBox() image.Rectangle {
	y0 := f.Y + LabelHeight
	return image.Rect(f.X, y0, f.X+f.W, y0+f.H)
}

// Region escala a caixa do valor para uma imagem de tamanho size, recuando
// inset pixels em cada lado. Arredonda meio para par, como o round do Python,
// para que os recortes sejam os mesmos da versão original.
func (f Field) Region(size image.Point, inset int) image.Rectangle {
	b := f.ValueBox()
	sx := float64(size.X) / PageWidth
	sy := float64(size.Y) / PageHeight
	scale := func(v int, s float64) int { return int(math.RoundToEven(float64(v) * s)) }
	return image.Rect(
		scale(b.Min.X+inset, sx), scale(b.Min.Y+inset, sy),
		scale(b.Max.X-inset, sx), scale(b.Max.Y-inset, sy),
	)
}
