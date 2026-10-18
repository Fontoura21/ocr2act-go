package main

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"

	"github.com/Fontoura21/ocr2act-go/internal/reference"
	"github.com/Fontoura21/ocr2act-go/internal/server"
	"github.com/Fontoura21/ocr2act-go/internal/web"
)

func serve(ctx context.Context, args []string, _, stderr io.Writer) int {
	fs := newFlagSet("servir", "servir [opções]", stderr)
	data := fs.String("dados", "data", "diretório do conjunto: docs/, resultados/, base.sqlite e verdade.json")
	addr := fs.String("addr", "", "endereço de escuta (padrão :8000, ou :$PORT quando definido)")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	log := slog.New(slog.NewTextHandler(stderr, nil))
	store, err := reference.Open(filepath.Join(*data, "base.sqlite"))
	if err != nil {
		return fail(stderr, err)
	}
	defer store.Close()
	srv, err := web.New(*data, store, log)
	if err != nil {
		return fail(stderr, err)
	}
	if err := server.Run(ctx, server.Addr(*addr, ":8000"), srv.Handler(), log); err != nil {
		return fail(stderr, err)
	}
	return 0
}
