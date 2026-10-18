package ocr

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	_ "image/png"
	"math"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// TSV real do Tesseract 5.5.3 para o NOME de R002_forte.
const tsv = "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
	"1\t1\t0\t0\t0\t0\t0\t0\t1110\t84\t-1\t\n" +
	"2\t1\t1\t0\t0\t0\t27\t28\t475\t26\t-1\t\n" +
	"3\t1\t1\t1\t0\t0\t27\t28\t475\t26\t-1\t\n" +
	"4\t1\t1\t1\t1\t0\t27\t28\t475\t26\t-1\t\n" +
	"5\t1\t1\t1\t1\t1\t27\t28\t126\t25\t95.959702\tARTHUR\n" +
	"5\t1\t1\t1\t1\t2\t162\t29\t129\t25\t96.001167\tGABRIEL\n" +
	"5\t1\t1\t1\t1\t3\t301\t29\t121\t25\t96.761436\tPASTOR\n" +
	"5\t1\t1\t1\t1\t4\t432\t30\t70\t24\t95.562874\tLIMA\n"

func TestParseTSV(t *testing.T) {
	got := ParseTSV([]byte(tsv))
	want := []Word{{"ARTHUR", 0.95}, {"GABRIEL", 0.96}, {"PASTOR", 0.96}, {"LIMA", 0.95}}
	if !slices.Equal(got, want) {
		t.Fatalf("ParseTSV = %v, want %v", got, want)
	}
	r := NewReading(got, "tesseract", "5.5.3")
	if r.Text != "ARTHUR GABRIEL PASTOR LIMA" || r.Confidence != 0.95 {
		t.Errorf("NewReading = %q %.2f", r.Text, r.Confidence)
	}
}

func TestParseTSVEdgeCases(t *testing.T) {
	header := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n"
	if got := ParseTSV([]byte(header)); len(got) != 0 || got == nil {
		t.Errorf("só o cabeçalho deveria dar lista vazia não nula, veio %#v", got)
	}
	// Na última linha o texto vazio some no strip e a célula é recomposta.
	trailing := header + "5\t1\t1\t1\t1\t1\t0\t0\t1\t1\t91.9\tCPF\n4\t1\t1\t1\t1\t0\t0\t0\t1\t1\t-1\t\n"
	if got := ParseTSV([]byte(trailing)); !slices.Equal(got, []Word{{"CPF", 0.91}}) {
		t.Errorf("última linha sem texto = %v", got)
	}
	bad := header + "5\t1\t1\t1\t1\t1\t0\t0\t1\t1\tnan\tX\n5\t1\t1\t1\t1\t2\t0\t0\t1\t1\tabc\tY\n"
	if got := ParseTSV([]byte(bad)); len(got) != 0 {
		t.Errorf("confiança inválida deveria ser ignorada, veio %v", got)
	}
}

func TestNewReading(t *testing.T) {
	r := NewReading([]Word{{"529.982.247-25", 0.91}, {"-", 0.10}}, "tesseract", "5.5.3")
	if r.Confidence != 0.91 {
		t.Errorf("token só de pontuação não pode baixar a confiança, veio %.2f", r.Confidence)
	}
	if r.Text != "529.982.247-25 -" {
		t.Errorf("Text = %q", r.Text)
	}
	empty := NewReading(nil, "tesseract", "5.5.3")
	if empty.Confidence != 0 || empty.Text != "" || empty.Words == nil {
		t.Errorf("leitura vazia = %#v", empty)
	}
}

func TestWordJSON(t *testing.T) {
	// Os resultados são gravados sem escape de HTML; a palavra não pode
	// reintroduzir o escape por conta própria.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode([]Word{{"A&B", 0.95}}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != `[["A&B",0.95]]` {
		t.Errorf("Encode = %s", got)
	}
	var back []Word
	if err := json.Unmarshal(buf.Bytes(), &back); err != nil || !slices.Equal(back, []Word{{"A&B", 0.95}}) {
		t.Errorf("Unmarshal = %v, %v", back, err)
	}
	var w Word
	if err := json.Unmarshal([]byte(`["só um"]`), &w); err == nil {
		t.Error("par incompleto deveria falhar")
	}
}

func TestGrayscaleMatchesPillow(t *testing.T) {
	// Casos conferidos contra Image.convert("L") do Pillow.
	tests := []struct {
		c    color.RGBA
		want uint8
	}{
		{color.RGBA{0, 0, 0, 255}, 0},
		{color.RGBA{255, 255, 255, 255}, 255},
		{color.RGBA{255, 0, 0, 255}, 76},
		{color.RGBA{0, 255, 0, 255}, 150},
		{color.RGBA{0, 0, 255, 255}, 29},
		{color.RGBA{12, 200, 99, 255}, 132},
	}
	img := image.NewRGBA(image.Rect(0, 0, len(tests), 1))
	for i, tt := range tests {
		img.SetRGBA(i, 0, tt.c)
	}
	g := Grayscale(img)
	for i, tt := range tests {
		if got := g.GrayAt(i, 0).Y; got != tt.want {
			t.Errorf("Grayscale(%v) = %d, want %d", tt.c, got, tt.want)
		}
	}
}

func TestGrayscaleOfDatasetPageIsIdenticalToPillow(t *testing.T) {
	img := loadPage(t, "R002_limpa.png")
	crop := Grayscale(img).SubImage(image.Rect(85, 361, 1155, 405)).(*image.Gray)
	h := sha256.New()
	for y := crop.Rect.Min.Y; y < crop.Rect.Max.Y; y++ {
		h.Write(crop.Pix[crop.PixOffset(crop.Rect.Min.X, y):crop.PixOffset(crop.Rect.Max.X, y)])
	}
	// sha256 de img.crop((85, 361, 1155, 405)).convert("L").tobytes() no Pillow.
	const want = "f81205eb9eaa43626022156a148aec0bbbc50203c9cd8b8e0f67f0af6cb05361"
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		t.Errorf("recorte em cinza difere do Pillow: sha256 %s, want %s", got, want)
	}
}

func TestPad(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 4, 3))
	for i := range src.Pix {
		src.Pix[i] = 7
	}
	sub := src.SubImage(image.Rect(1, 1, 3, 3)).(*image.Gray)
	p := Pad(sub, 2, 0xff)
	if p.Bounds() != image.Rect(0, 0, 6, 6) {
		t.Fatalf("Bounds = %v", p.Bounds())
	}
	if p.GrayAt(0, 0).Y != 0xff || p.GrayAt(2, 2).Y != 7 || p.GrayAt(3, 3).Y != 7 || p.GrayAt(4, 4).Y != 0xff {
		t.Errorf("margem ou conteúdo fora do lugar: %v", p.Pix)
	}
}

func TestSharpness(t *testing.T) {
	flat := image.NewGray(image.Rect(0, 0, 10, 10))
	if got := Sharpness(flat); got != 0 {
		t.Errorf("imagem lisa = %v, want 0", got)
	}
	if got := Sharpness(image.NewGray(image.Rect(0, 0, 2, 2))); got != 0 {
		t.Errorf("imagem menor que o núcleo = %v, want 0", got)
	}
	// Um único pixel claro no centro de 5x5: os Laplacianos internos são
	// -400 (centro), 100 (quatro vizinhos) e 0 (quatro cantos internos).
	// A média é zero, então a variância é a média dos quadrados.
	dot := image.NewGray(image.Rect(0, 0, 5, 5))
	dot.SetGray(2, 2, color.Gray{Y: 100})
	want := (400.0*400 + 4*100*100) / 9
	if got := Sharpness(dot); math.Abs(got-want) > 1e-9 {
		t.Errorf("Sharpness = %v, want %v", got, want)
	}
}

func TestParseVersion(t *testing.T) {
	tests := map[string]string{
		"tesseract 5.5.3\n leptonica-1.87.0\n": "5.5.3",
		"tesseract v5.3.0-dev\n":               "5.3.0",
	}
	for in, want := range tests {
		if got, err := parseVersion([]byte(in)); err != nil || got != want {
			t.Errorf("parseVersion(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := parseVersion([]byte("outra coisa")); err == nil {
		t.Error("parseVersion deveria falhar com saída inesperada")
	}
}

func TestTesseractReadsDatasetRegion(t *testing.T) {
	if _, err := exec.LookPath("tesseract"); err != nil {
		t.Skip("tesseract não está instalado")
	}
	engine, err := NewTesseract(context.Background(), "../../tessdata")
	if err != nil {
		t.Fatal(err)
	}
	img := Grayscale(loadPage(t, "R002_limpa.png"))
	r, err := engine.Read(context.Background(), img.SubImage(image.Rect(85, 361, 1155, 405)).(*image.Gray))
	if err != nil {
		t.Fatal(err)
	}
	if r.Text != "ARTHUR GABRIEL PASTOR LIMA" || r.Engine != "tesseract" || r.Version == "" {
		t.Errorf("Read = %+v", r)
	}
	t.Logf("tesseract %s: %q com confiança %.2f", r.Version, r.Text, r.Confidence)
}

func loadPage(t *testing.T, name string) image.Image {
	t.Helper()
	f, err := os.Open("../../data/docs/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}
