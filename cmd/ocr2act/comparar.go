package main

import (
	"context"
	"fmt"
	"io"

	"github.com/Fontoura21/ocr2act-go/internal/parity"
	"github.com/Fontoura21/ocr2act-go/internal/pipeline"
)

// compare sai com 0 se os resultados têm os mesmos textos e decisões, 1 se
// diferem, como o diff.
func compare(_ context.Context, args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("comparar", "comparar <resultados_a> <resultados_b>", stderr)
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if fs.NArg() != 2 {
		fs.Usage()
		return 2
	}
	a, err := pipeline.ReadResults(fs.Arg(0))
	if err != nil {
		return fail(stderr, err)
	}
	b, err := pipeline.ReadResults(fs.Arg(1))
	if err != nil {
		return fail(stderr, err)
	}
	rep := parity.Compare(a, b)
	pct := func(n int) string {
		if rep.Fields == 0 {
			return "-"
		}
		return fmt.Sprintf("%.1f%%", 100*float64(n)/float64(rep.Fields))
	}
	fmt.Fprintf(stdout, "documentos comparados: %d (só em A: %d, só em B: %d)\n", rep.Documents, len(rep.OnlyA), len(rep.OnlyB))
	fmt.Fprintf(stdout, "campos comparados:     %d\n", rep.Fields)
	fmt.Fprintf(stdout, "texto lido idêntico:   %d (%s)\n", rep.SameText, pct(rep.SameText))
	fmt.Fprintf(stdout, "confiança idêntica:    %d (%s)\n", rep.SameConfidence, pct(rep.SameConfidence))
	fmt.Fprintf(stdout, "decisão idêntica:      %d (%s)\n", rep.SameDecision, pct(rep.SameDecision))
	for _, d := range rep.Differences {
		fmt.Fprintf(stdout, "\n%s.%s\n  A: %q %.2f %s/%s\n  B: %q %.2f %s/%s\n", d.DocID, d.Field,
			d.A.Text, d.A.Confidence, d.A.Class, d.A.Rule, d.B.Text, d.B.Confidence, d.B.Class, d.B.Rule)
	}
	if !rep.Identical() {
		return 1
	}
	return 0
}
