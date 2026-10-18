// Comando referencia sobe só a API de referência (Mini-CIC sintético), como
// serviço independente. Escuta em :$PORT quando a variável existe e grava logs
// em JSON no formato do Cloud Logging, então roda no Cloud Run sem ajustes.
//
// Uso:
//
//	referencia [-base data/base.sqlite] [-addr :8080]
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/Fontoura21/ocr2act-go/internal/reference"
	"github.com/Fontoura21/ocr2act-go/internal/server"
)

func main() {
	base := flag.String("base", "data/base.sqlite", "base SQLite de referência")
	addr := flag.String("addr", "", "endereço de escuta (padrão :8080, ou :$PORT quando definido)")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, *base, server.Addr(*addr, ":8080"))
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, base, addr string) error {
	log := server.CloudLogger(os.Stdout)
	store, err := reference.Open(base)
	if err != nil {
		return err
	}
	defer store.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.Health)
	reference.Routes(mux, store, log)
	return server.Run(ctx, addr, mux, log)
}
