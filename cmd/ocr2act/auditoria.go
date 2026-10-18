package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/Fontoura21/ocr2act-go/internal/audit"
)

func verifyAudit(_ context.Context, args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("auditoria", "auditoria [opções]", stderr)
	var p paths
	p.register(fs)
	if code, ok := parse(fs, args); !ok {
		return code
	}
	v, err := audit.New(filepath.Join(p.data, "auditoria.jsonl")).Verify()
	if err != nil {
		return fail(stderr, err)
	}
	if !v.OK {
		fmt.Fprintln(stdout, "FALHA:", v.Message)
		return 1
	}
	fmt.Fprintln(stdout, "OK:", v.Message)
	return 0
}
