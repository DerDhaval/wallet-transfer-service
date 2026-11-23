.PHONY: up test cover run tidy

DATABASE_URL ?= postgres://wallet:wallet@127.0.0.1:5433/wallet_transfer?sslmode=disable

up:
	docker-compose up -d postgres

test:
	DATABASE_URL="$(DATABASE_URL)" go test -race -count=1 ./...

cover:
	DATABASE_URL="$(DATABASE_URL)" go test -race -count=1 -coverpkg=./internal/...,./pkg/... -coverprofile=coverage.out ./internal/... ./pkg/...
	go tool cover -func=coverage.out | tail -1

run:
	DATABASE_URL="$(DATABASE_URL)" go run ./cmd/server

tidy:
	go mod tidy

# Optional: inspect applied versions
# psql "$(DATABASE_URL)" -c 'SELECT * FROM schema_migrations ORDER BY version'
