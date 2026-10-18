package ocr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Fontoura21/ocr2act-go/internal/normalize"
)

// Padding é a margem branca em volta do recorte. No protótipo em Python, ampliar o
// recorte piorou o CER no teste da Semana 2; a margem ficou.
const Padding = 20

var pngEncoder = png.Encoder{CompressionLevel: png.BestSpeed}

// Tesseract chama o binário do Tesseract em modo de linha única (--psm 7).
// Cada leitura é um processo, então leituras concorrentes são seguras.
type Tesseract struct {
	bin      string
	tessdata string
	lang     string
	version  string
}

// NewTesseract localiza o binário e confere o modelo de português em
// tessdataDir.
func NewTesseract(ctx context.Context, tessdataDir string) (*Tesseract, error) {
	bin, err := exec.LookPath("tesseract")
	if err != nil {
		return nil, fmt.Errorf("ocr: tesseract não encontrado no PATH: %w", err)
	}
	dir, err := filepath.Abs(tessdataDir)
	if err != nil {
		return nil, fmt.Errorf("ocr: %w", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "por.traineddata")); err != nil {
		return nil, fmt.Errorf("ocr: modelo do português: %w", err)
	}
	out, err := exec.CommandContext(ctx, bin, "--version").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ocr: tesseract --version: %w", err)
	}
	version, err := parseVersion(out)
	if err != nil {
		return nil, err
	}
	return &Tesseract{bin: bin, tessdata: dir, lang: "por", version: version}, nil
}

// Version é a versão do motor, registrada na proveniência de cada campo.
func (t *Tesseract) Version() string { return t.version }

// Read lê uma linha de texto da região. A imagem vai pela entrada padrão e o
// TSV volta pela saída padrão, sem arquivos temporários.
func (t *Tesseract) Read(ctx context.Context, region *image.Gray) (Reading, error) {
	var in bytes.Buffer
	if err := pngEncoder.Encode(&in, Pad(region, Padding, 0xff)); err != nil {
		return Reading{}, fmt.Errorf("ocr: %w", err)
	}
	cmd := exec.CommandContext(ctx, t.bin, "stdin", "stdout",
		"-l", t.lang, "--psm", "7", "--tessdata-dir", t.tessdata,
		"-c", "tessedit_create_tsv=1", "-c", "tessedit_create_txt=0")
	cmd.Stdin = &in
	// Um processo por região já usa todos os núcleos; threads internas do
	// Tesseract só disputariam CPU com as outras leituras.
	cmd.Env = append(os.Environ(), "OMP_THREAD_LIMIT=1")
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Reading{}, ctxErr
		}
		return Reading{}, fmt.Errorf("ocr: tesseract: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return NewReading(ParseTSV(out.Bytes()), "tesseract", t.version), nil
}

// ParseTSV extrai as palavras do TSV do Tesseract. Segue o pytesseract: a
// confiança é truncada para inteiro antes de virar fração (95.96 vira 0.95) e
// as linhas de nível de página, bloco e parágrafo, com confiança -1, saem.
func ParseTSV(tsv []byte) []Word {
	lines := strings.Split(strings.TrimFunc(string(tsv), normalize.IsSpace), "\n")
	words := []Word{}
	if len(lines) < 2 {
		return words
	}
	header := strings.Split(lines[0], "\t")
	confCol := -1
	for i, h := range header {
		if h == "conf" {
			confCol = i
		}
	}
	textCol := len(header) - 1
	if confCol < 0 {
		return words
	}
	rows := lines[1:]
	for i, line := range rows {
		cells := strings.Split(line, "\t")
		// O texto vazio da última linha some no strip do TSV inteiro.
		if i == len(rows)-1 && len(cells) < len(header) {
			cells = append(cells, "")
		}
		if len(cells) <= textCol {
			continue
		}
		conf, err := strconv.ParseFloat(strings.TrimSpace(cells[confCol]), 64)
		if err != nil || math.IsNaN(conf) || math.IsInf(conf, 0) {
			continue
		}
		conf = math.Trunc(conf)
		text := strings.TrimFunc(cells[textCol], normalize.IsSpace)
		if text == "" || conf < 0 {
			continue
		}
		words = append(words, Word{Text: text, Confidence: conf / 100})
	}
	return words
}

// parseVersion lê "tesseract 5.5.3" da primeira linha de --version.
func parseVersion(out []byte) (string, error) {
	first, _, _ := strings.Cut(string(out), "\n")
	fields := strings.Fields(first)
	if len(fields) < 2 || fields[0] != "tesseract" {
		return "", errors.New("ocr: saída inesperada de tesseract --version: " + strings.TrimSpace(first))
	}
	version, _, _ := strings.Cut(strings.TrimPrefix(fields[1], "v"), "-")
	return version, nil
}
