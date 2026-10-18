package web

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// Tamanho máximo da imagem de sobreposição exibida ao lado dos campos.
const maxOverlayW, maxOverlayH = 760, 1000

// overlay desenha as regiões lidas, coloridas pela decisão de cada campo.
func (s *Server) overlay(w http.ResponseWriter, r *http.Request) {
	res, ok := s.result(w, r)
	if !ok {
		return
	}
	img, ok := s.loadImage(w, r, res.File)
	if !ok {
		return
	}
	canvas := image.NewRGBA(img.Bounds())
	xdraw.Draw(canvas, canvas.Bounds(), img, img.Bounds().Min, xdraw.Src)
	for _, f := range res.Fields {
		box := image.Rect(f.BBox[0], f.BBox[1], f.BBox[2], f.BBox[3])
		outline(canvas, box, 4, classOf(f.Decision.Class).rgba)
	}
	s.writePNG(w, r, fit(canvas, maxOverlayW, maxOverlayH))
}

// crop devolve a região lida de um campo, em cores e sem a margem branca
// acrescentada para o OCR.
func (s *Server) crop(w http.ResponseWriter, r *http.Request) {
	res, ok := s.result(w, r)
	if !ok {
		return
	}
	name, isPNG := strings.CutSuffix(r.PathValue("file"), ".png")
	f, found := res.Fields.Get(name)
	if !isPNG || !found {
		http.NotFound(w, r)
		return
	}
	img, ok := s.loadImage(w, r, res.File)
	if !ok {
		return
	}
	sub, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		s.fail(w, r, errors.New("imagem sem recorte"))
		return
	}
	s.writePNG(w, r, sub.SubImage(image.Rect(f.BBox[0], f.BBox[1], f.BBox[2], f.BBox[3])))
}

// loadImage abre a imagem original do documento. O nome vem do resultado e é
// reduzido ao nome do arquivo, para nunca sair do diretório das certidões.
func (s *Server) loadImage(w http.ResponseWriter, r *http.Request, file string) (image.Image, bool) {
	f, err := os.Open(filepath.Join(s.docs, filepath.Base(file)))
	if errors.Is(err, fs.ErrNotExist) {
		http.Error(w, "imagem do documento não encontrada", http.StatusNotFound)
		return nil, false
	}
	if err != nil {
		s.fail(w, r, err)
		return nil, false
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		s.fail(w, r, err)
		return nil, false
	}
	return img, true
}

// outline desenha o contorno de box com a espessura dada, para dentro, como o
// rectangle(width=...) do Pillow. As bordas direita e inferior são inclusivas.
func outline(dst *image.RGBA, box image.Rectangle, width int, c color.RGBA) {
	x0, y0, x1, y1 := box.Min.X, box.Min.Y, box.Max.X, box.Max.Y
	for i := range width {
		for x := x0; x <= x1; x++ {
			dst.SetRGBA(x, y0+i, c)
			dst.SetRGBA(x, y1-i, c)
		}
		for y := y0; y <= y1; y++ {
			dst.SetRGBA(x0+i, y, c)
			dst.SetRGBA(x1-i, y, c)
		}
	}
}

// fit reduz a imagem para caber em maxW x maxH mantendo a proporção. Imagens
// menores voltam como estão.
func fit(img image.Image, maxW, maxH int) image.Image {
	b := img.Bounds()
	scale := min(float64(maxW)/float64(b.Dx()), float64(maxH)/float64(b.Dy()))
	if scale >= 1 {
		return img
	}
	w := max(1, int(float64(b.Dx())*scale+0.5))
	h := max(1, int(float64(b.Dy())*scale+0.5))
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, xdraw.Src, nil)
	return dst
}

func (s *Server) writePNG(w http.ResponseWriter, r *http.Request, img image.Image) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = buf.WriteTo(w)
}
