STATICCHECK := honnef.co/go/tools/cmd/staticcheck@2026.2.2
GOVULNCHECK := golang.org/x/vuln/cmd/govulncheck@v1.9.0

.PHONY: build test lint vuln processar metricas auditoria servir referencia docker clean help

build: ## compila ocr2act e referencia em bin/
	go build -o bin/ ./cmd/...

test: ## roda os testes com o detector de corridas
	go test -race ./...

lint: ## formatação, go vet e staticcheck
	@test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; }
	go vet ./...
	go run $(STATICCHECK) ./...

vuln: ## procura vulnerabilidades conhecidas nas dependências
	go run $(GOVULNCHECK) ./...

processar: ## lê as 90 certidões e grava data/resultados
	go run ./cmd/ocr2act processar

metricas: ## mede as decisões contra a verdade de referência
	go run ./cmd/ocr2act metricas

auditoria: ## verifica a trilha de auditoria
	go run ./cmd/ocr2act auditoria

servir: ## interface de revisão em http://localhost:8000
	go run ./cmd/ocr2act servir

referencia: ## só a API de referência em http://localhost:8080
	go run ./cmd/referencia

docker: ## constrói as duas imagens
	docker build -t ocr2act .
	docker build --target referencia -t ocr2act-referencia .

clean: ## apaga binários e saídas geradas
	rm -rf bin data/resultados data/metricas.json data/auditoria.jsonl

help: ## lista os alvos
	@grep -E '^[a-z]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "} {printf "  %-12s %s\n", $$1, $$2}'
