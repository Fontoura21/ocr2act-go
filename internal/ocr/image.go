package ocr

import (
	"image"
	"image/color"
)

// Grayscale converte a imagem para tons de cinza com a mesma conta inteira do
// Pillow (ITU-R 601-2), para que os recortes enviados ao OCR tenham os mesmos
// pixels do protótipo em Python.
func Grayscale(img image.Image) *image.Gray {
	b := img.Bounds()
	dst := image.NewGray(b)
	switch src := img.(type) {
	case *image.Gray:
		for y := b.Min.Y; y < b.Max.Y; y++ {
			copy(dst.Pix[dst.PixOffset(b.Min.X, y):dst.PixOffset(b.Max.X, y)], src.Pix[src.PixOffset(b.Min.X, y):])
		}
	case *image.YCbCr:
		for y := b.Min.Y; y < b.Max.Y; y++ {
			o := dst.PixOffset(b.Min.X, y)
			for x := b.Min.X; x < b.Max.X; x++ {
				ci := src.COffset(x, y)
				r, g, bl := color.YCbCrToRGB(src.Y[src.YOffset(x, y)], src.Cb[ci], src.Cr[ci])
				dst.Pix[o] = luma(r, g, bl)
				o++
			}
		}
	case *image.RGBA:
		for y := b.Min.Y; y < b.Max.Y; y++ {
			o := dst.PixOffset(b.Min.X, y)
			for x := b.Min.X; x < b.Max.X; x++ {
				p := src.Pix[src.PixOffset(x, y):]
				if p[3] == 0xff {
					dst.Pix[o] = luma(p[0], p[1], p[2])
				} else {
					c := color.NRGBAModel.Convert(src.RGBAAt(x, y)).(color.NRGBA)
					dst.Pix[o] = luma(c.R, c.G, c.B)
				}
				o++
			}
		}
	default:
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
				dst.SetGray(x, y, color.Gray{Y: luma(c.R, c.G, c.B)})
			}
		}
	}
	return dst
}

// luma é o L = R*299/1000 + G*587/1000 + B*114/1000 do Pillow, em ponto fixo
// de 16 bits com arredondamento.
func luma(r, g, b uint8) uint8 {
	return uint8((uint32(r)*19595 + uint32(g)*38470 + uint32(b)*7471 + 0x8000) >> 16)
}

// Pad devolve uma cópia da imagem com uma margem de border pixels na cor
// fill. Uma margem branca em volta da linha melhora a leitura do Tesseract.
func Pad(src *image.Gray, border int, fill uint8) *image.Gray {
	b := src.Bounds()
	dst := image.NewGray(image.Rect(0, 0, b.Dx()+2*border, b.Dy()+2*border))
	for i := range dst.Pix {
		dst.Pix[i] = fill
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row := src.Pix[src.PixOffset(b.Min.X, y):src.PixOffset(b.Max.X, y)]
		copy(dst.Pix[dst.PixOffset(border, y-b.Min.Y+border):], row)
	}
	return dst
}

// Sharpness é a variância do Laplaciano: quanto maior, mais nítida a captura.
// Usa somas inteiras exatas, então o resultado não depende da ordem de soma.
func Sharpness(g *image.Gray) float64 {
	b := g.Bounds()
	if b.Dx() < 3 || b.Dy() < 3 {
		return 0
	}
	at := func(x, y int) int64 { return int64(g.Pix[g.PixOffset(x, y)]) }
	var n, sum, sumSq int64
	for y := b.Min.Y + 1; y < b.Max.Y-1; y++ {
		for x := b.Min.X + 1; x < b.Max.X-1; x++ {
			lap := at(x, y-1) + at(x, y+1) + at(x-1, y) + at(x+1, y) - 4*at(x, y)
			sum += lap
			sumSq += lap * lap
			n++
		}
	}
	mean := float64(sum) / float64(n)
	return float64(sumSq)/float64(n) - mean*mean
}
