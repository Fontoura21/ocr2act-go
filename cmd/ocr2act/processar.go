package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Fontoura21/ocr2act-go/internal/config"
	"github.com/Fontoura21/ocr2act-go/internal/ocr"
	"github.com/Fontoura21/ocr2act-go/internal/pipeline"
	"github.com/Fontoura21/ocr2act-go/internal/reference"
	"github.com/Fontoura21/ocr2act-go/internal/rules"
)

func process(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("processar", "processar [opções] [doc_id ...]", stderr)
	var p paths
	p.register(fs)
	workers := fs.Int("workers", runtime.NumCPU(), "documentos processados ao mesmo tempo")
	refURL := fs.String("referencia", "", "URL da API de referência; vazio usa <dados>/base.sqlite")
	out := fs.String("saida", "", "diretório dos resultados (padrão <dados>/resultados)")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if *workers < 1 {
		fmt.Fprintln(stderr, "-workers deve ser pelo menos 1")
		return 2
	}
	if *out == "" {
		*out = filepath.Join(p.data, "resultados")
	}

	cfg, err := config.Load(p.config)
	if err != nil {
		return fail(stderr, err)
	}
	engine, err := ocr.NewTesseract(ctx, p.tessdata)
	if err != nil {
		return fail(stderr, err)
	}
	var src reference.Source
	if *refURL != "" {
		if src, err = reference.NewClient(*refURL, nil); err != nil {
			return fail(stderr, err)
		}
	} else {
		store, err := reference.Open(filepath.Join(p.data, "base.sqlite"))
		if err != nil {
			return fail(stderr, err)
		}
		defer store.Close()
		src = store
	}
	docs, err := pipeline.Documents(filepath.Join(p.data, "docs"), fs.Args()...)
	if err != nil {
		return fail(stderr, err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return fail(stderr, err)
	}

	start := time.Now()
	done, failed := 0, 0
	for o := range pipeline.New(cfg, engine, src).ProcessAll(ctx, docs, *workers) {
		done++
		if o.Err == nil {
			o.Err = pipeline.WriteResult(*out, o.Result)
		}
		if o.Err != nil {
			failed++
			fmt.Fprintf(stderr, "[%3d/%d] %s: %v\n", done, len(docs), filepath.Base(o.Path), o.Err)
			continue
		}
		r := o.Result
		alert := ""
		if len(r.Alerts) > 0 {
			alert = " ALERTA"
		}
		fmt.Fprintf(stdout, "[%3d/%d] %-12s %5.2fs nitidez=%7.1f %s%s\n",
			done, len(docs), r.DocID, r.Seconds, r.Sharpness, summary(r), alert)
	}
	if ctx.Err() != nil {
		fmt.Fprintf(stderr, "interrompido depois de %d de %d documento(s)\n", done, len(docs))
		return 1
	}
	fmt.Fprintf(stdout, "%d documento(s) em %.1f s com %d worker(s); resultados em %s\n",
		done-failed, time.Since(start).Seconds(), *workers, *out)
	if failed > 0 {
		fmt.Fprintf(stderr, "%d documento(s) com erro\n", failed)
		return 1
	}
	return 0
}

// summary conta os campos por classe, como a saída do protótipo em Python.
func summary(r *pipeline.Result) string {
	counts := map[rules.Class]int{}
	for _, f := range r.Fields {
		counts[f.Decision.Class]++
	}
	parts := make([]string, len(rules.Classes))
	for i, c := range rules.Classes {
		parts[i] = fmt.Sprintf("%s=%d", c, counts[c])
	}
	return strings.Join(parts, " ")
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, "erro:", err)
	return 1
}
