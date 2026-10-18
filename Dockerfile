# syntax=docker/dockerfile:1

# Duas imagens a partir do mesmo build:
#   docker build -t ocr2act .                                  aplicação completa (padrão)
#   docker build --target referencia -t ocr2act-referencia .   só a API de referência

ARG GO_VERSION=1.27

FROM golang:${GO_VERSION}-trixie AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
# Sem cgo: o SQLite é Go puro, então os binários são estáticos.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/...

# Serviço de referência: binário estático e a base, sem shell nem pacotes.
# Escuta em :$PORT, como o Cloud Run espera.
FROM gcr.io/distroless/static-debian13:nonroot AS referencia
COPY --from=build /out/referencia /referencia
COPY data/base.sqlite /data/base.sqlite
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/referencia", "-base", "/data/base.sqlite"]

# Aplicação completa: CLI, interface de revisão e o Tesseract.
FROM debian:trixie-slim AS app
RUN apt-get update \
 && apt-get install -y --no-install-recommends tesseract-ocr \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --system --create-home --uid 10001 ocr2act
WORKDIR /app
COPY --from=build /out/ocr2act /usr/local/bin/ocr2act
COPY config ./config
COPY tessdata ./tessdata
COPY --chown=ocr2act data ./data
USER ocr2act
ENV PORT=8000
EXPOSE 8000
# Processa o conjunto e sobe a interface; qualquer outro comando substitui este.
CMD ["sh", "-c", "ocr2act processar && exec ocr2act servir"]
