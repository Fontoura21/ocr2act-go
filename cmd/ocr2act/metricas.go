package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/Fontoura21/ocr2act-go/internal/dataset"
	"github.com/Fontoura21/ocr2act-go/internal/metrics"
	"github.com/Fontoura21/ocr2act-go/internal/pipeline"
	"github.com/Fontoura21/ocr2act-go/internal/rules"
)

func measure(_ context.Context, args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("metricas", "metricas [opções]", stderr)
	var p paths
	p.register(fs)
	results := fs.String("resultados", "", "diretório dos resultados (padrão <dados>/resultados)")
	out := fs.String("saida", "", "arquivo do relatório (padrão <dados>/metricas.json)")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if *results == "" {
		*results = filepath.Join(p.data, "resultados")
	}
	if *out == "" {
		*out = filepath.Join(p.data, "metricas.json")
	}
	rs, err := pipeline.ReadResults(*results)
	if err != nil {
		return fail(stderr, err)
	}
	if len(rs) == 0 {
		return fail(stderr, fmt.Errorf("nenhum resultado em %s; rode ocr2act processar antes", *results))
	}
	truth, err := dataset.Load(filepath.Join(p.data, "verdade.json"))
	if err != nil {
		return fail(stderr, err)
	}
	report, err := metrics.Compute(rs, truth)
	if err != nil {
		return fail(stderr, err)
	}
	if err := metrics.Write(*out, report); err != nil {
		return fail(stderr, err)
	}
	for _, s := range report {
		printSplit(stdout, s)
	}
	fmt.Fprintf(stdout, "\nrelatório gravado em %s\n", *out)
	return 0
}

func printSplit(w io.Writer, s metrics.Split) {
	fmt.Fprintf(w, "\n== %s (%d campos)\n", s.Name, s.Fields)
	fmt.Fprintf(w, "  falso compatível crítico: %d  %v\n", s.FalseCompatibleCritical, s.FalseCompatibleCases)
	fmt.Fprintf(w, "  revocação de divergência real: %s\n", optional(s.RealDivergenceRecall))
	fmt.Fprintf(w, "  decisões aceitáveis: %s\n", optional(s.AcceptableDecisions))
	cer := make([]string, len(s.CERByLevel))
	for i, kv := range s.CERByLevel {
		cer[i] = fmt.Sprintf("%s=%g", kv.Key, kv.Value)
	}
	fmt.Fprintf(w, "  CER por nível: %s\n", strings.Join(cer, " "))
	header := make([]string, len(rules.Classes))
	for i, c := range rules.Classes {
		header[i] = string(c)
	}
	fmt.Fprintf(w, "  matriz (verdade x classe): %s\n", strings.Join(header, " | "))
	for _, row := range s.Matrix {
		fmt.Fprintf(w, "    %-18s", row.Key)
		for _, cell := range row.Value {
			fmt.Fprintf(w, " %5d", cell.Value)
		}
		fmt.Fprintln(w)
	}
}

func optional(v *float64) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%g", *v)
}
