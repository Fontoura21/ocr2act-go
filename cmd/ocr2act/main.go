// Comando ocr2act: extração por OCR, validação cruzada e revisão de certidões
// de nascimento sintéticas.
//
// Uso:
//
//	ocr2act processar [-workers N] [-referencia URL] [doc_id ...]
//	ocr2act metricas
//	ocr2act auditoria
//	ocr2act comparar <resultados_a> <resultados_b>
//	ocr2act servir [-addr :8000]
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

const usage = `ocr2act: extração por OCR, validação cruzada e revisão de certidões sintéticas.

Uso:
  ocr2act processar [-workers N] [-referencia URL] [doc_id ...]
      lê as certidões de <dados>/docs e grava um JSON por documento
  ocr2act metricas
      mede as decisões contra a verdade de referência
  ocr2act auditoria
      verifica a integridade da trilha de auditoria
  ocr2act comparar <resultados_a> <resultados_b>
      compara dois conjuntos de resultados campo a campo
  ocr2act servir [-addr :8000]
      interface de revisão humana e API de referência

Use "ocr2act <comando> -h" para ver as opções de cada comando.
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// run executa o comando e devolve o código de saída: 0 sucesso, 1 falha,
// 2 uso incorreto.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	commands := map[string]func(context.Context, []string, io.Writer, io.Writer) int{
		"processar": process,
		"metricas":  measure,
		"auditoria": verifyAudit,
		"comparar":  compare,
		"servir":    serve,
	}
	cmd, ok := commands[args[0]]
	switch {
	case ok:
		return cmd(ctx, args[1:], stdout, stderr)
	case args[0] == "-h" || args[0] == "-help" || args[0] == "--help" || args[0] == "ajuda":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "comando desconhecido: %s\n\n%s", args[0], usage)
		return 2
	}
}

// paths são os caminhos comuns aos comandos, relativos à raiz do repositório.
type paths struct {
	data, config, tessdata string
}

func (p *paths) register(fs *flag.FlagSet) {
	fs.StringVar(&p.data, "dados", "data", "diretório do conjunto: docs/, base.sqlite e verdade.json")
	fs.StringVar(&p.config, "config", "config/regras.yaml", "arquivo de regras e limiares")
	fs.StringVar(&p.tessdata, "tessdata", "tessdata", "diretório com por.traineddata")
}

// parse lê as flags do comando. ok falso significa que o processo deve sair
// com o código devolvido (0 para -h, 2 para flag inválida).
func parse(fs *flag.FlagSet, args []string) (code int, ok bool) {
	switch err := fs.Parse(args); {
	case err == flag.ErrHelp:
		return 0, false
	case err != nil:
		return 2, false
	}
	return 0, true
}

func newFlagSet(name, synopsis string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Uso: ocr2act %s\n\nOpções:\n", synopsis)
		fs.PrintDefaults()
	}
	return fs
}
