.PHONY: up test cover run tidy

# Load .env if present (copy from .env.example).
ifneq (,$(wildcard .env))
include .env
export
endif

DATABASE_URL ?= postgres://wallet:wallet@127.0.0.1:5433/wallet_transfer?sslmode=disable
HTTP_ADDR ?= :8080
LOG_LEVEL ?= info

up:
	docker-compose up -d postgres

test:
	DATABASE_URL="$(DATABASE_URL)" go test -race -count=1 ./...

cover:
	DATABASE_URL="$(DATABASE_URL)" go test -race -count=1 -coverpkg=./internal/...,./pkg/... -coverprofile=coverage.out ./internal/... ./pkg/...
	go tool cover -func=coverage.out | tail -1

run:
	DATABASE_URL="$(DATABASE_URL)" HTTP_ADDR="$(HTTP_ADDR)" LOG_LEVEL="$(LOG_LEVEL)" go run ./cmd/server

tidy:
	go mod tidy

# Optional: inspect applied versions
# psql "$(DATABASE_URL)" -c 'SELECT * FROM schema_migrations ORDER BY version'
