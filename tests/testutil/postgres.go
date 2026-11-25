// Package testutil holds shared helpers for Postgres integration tests:
// private DB setup, truncate, balance/ledger assertions, and common JSON checks.
package testutil

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"wallet-transfer-service/pkg/database"
)

// Pool is the migrated database for the current test process.
var Pool *pgxpool.Pool

// Run creates a private database, migrates it, and runs the tests.
func Run(m *testing.M, name string) int {
	if err := safeIdent(name); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	adminURL := databaseURL()
	dbName := "wts_test_" + name

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "postgres is required (%s): %v\nstart it with: docker-compose up -d postgres\n", adminURL, err)
		return 1
	}
	defer admin.Close()
	if err := admin.Ping(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "postgres is required (%s): %v\nstart it with: docker-compose up -d postgres\n", adminURL, err)
		return 1
	}

	if err := recreate(ctx, admin, dbName); err != nil {
		fmt.Fprintf(os.Stderr, "create test database %s: %v\n", dbName, err)
		return 1
	}

	testURL, err := withDatabase(adminURL, dbName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cfg, err := pgxpool.ParseConfig(testURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cfg.MaxConns = 40

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect test database: %v\n", err)
		return 1
	}
	if err := database.Migrate(ctx, pool); err != nil {
		pool.Close()
		fmt.Fprintf(os.Stderr, "migrate test database: %v\n", err)
		_ = drop(context.Background(), admin, dbName)
		return 1
	}

	Pool = pool
	code := m.Run()
	pool.Close()
	Pool = nil
	if err := drop(context.Background(), admin, dbName); err != nil {
		fmt.Fprintf(os.Stderr, "drop test database %s: %v\n", dbName, err)
	}
	return code
}

// RequirePostgres returns the migrated pool.
func RequirePostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if Pool == nil {
		t.Fatal("postgres test pool was not initialized")
	}
	return Pool
}

// Clean removes financial rows so tests do not depend on each other's data.
func Clean(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `TRUNCATE ledger_entries, idempotency_records, transfers, wallets CASCADE`)
	if err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func databaseURL() string {
	if value := os.Getenv("DATABASE_URL"); value != "" {
		return value
	}
	return "postgres://wallet:wallet@127.0.0.1:5433/wallet_transfer?sslmode=disable"
}

func recreate(ctx context.Context, admin *pgxpool.Pool, dbName string) error {
	if err := drop(ctx, admin, dbName); err != nil {
		return err
	}
	_, err := admin.Exec(ctx, `CREATE DATABASE `+dbName)
	return err
}

func drop(ctx context.Context, admin *pgxpool.Pool, dbName string) error {
	if err := safeIdent(dbName); err != nil {
		return err
	}
	rows, err := admin.Query(ctx, `
		SELECT pg_terminate_backend(pid)
		FROM pg_stat_activity
		WHERE datname = $1 AND pid <> pg_backend_pid()`, dbName)
	if err != nil {
		return err
	}
	rows.Close()
	_, err = admin.Exec(ctx, `DROP DATABASE IF EXISTS `+dbName)
	return err
}

func withDatabase(rawURL, dbName string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	parsed.Path = "/" + dbName
	return parsed.String(), nil
}

func safeIdent(name string) error {
	if name == "" {
		return fmt.Errorf("empty database identifier")
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return fmt.Errorf("unsafe database identifier %q", name)
		}
	}
	return nil
}
