# Estágio 1: Build
FROM golang:1.22-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum* ./
RUN go mod download || true

COPY . .

RUN go mod tidy && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /app/bin/finsense-api ./cmd/api

# Estágio 2: Imagem Final Mínima e Segura
FROM alpine:3.19

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata curl && \
    addgroup -S finsense && adduser -S finsense -G finsense

COPY --from=builder /app/bin/finsense-api /app/finsense-api
COPY --from=builder /app/docs /app/docs

RUN mkdir -p /app/models && chown -R finsense:finsense /app

USER finsense

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/finsense-api"]
