package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"wallet-transfer-service/pkg/database"
	"wallet-transfer-service/tests/testutil"
)

func TestMain(m *testing.M) {
	os.Exit(testutil.Run(m, "database"))
}

func TestConnect(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "valid", url: envOrDefault()},
		{name: "bad url", url: "postgres://bad:bad@127.0.0.1:1/nope?sslmode=disable", wantErr: true},
		{name: "unparseable", url: "://not-a-url", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			pool, err := database.Connect(ctx, tc.url)
			if tc.wantErr {
				if err == nil {
					pool.Close()
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			pool.Close()
		})
	}
}

func TestMigrateUpAndDown(t *testing.T) {
	pool := testutil.RequirePostgres(t)
	ctx := context.Background()

	// TestMain already migrated. Re-running must be a no-op.
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	var ups int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&ups); err != nil {
		t.Fatal(err)
	}
	if ups != 4 {
		t.Fatalf("applied = %d, want 4 table migrations", ups)
	}

	for want := 3; want >= 0; want-- {
		if err := database.MigrateDown(ctx, pool); err != nil {
			t.Fatalf("down to %d: %v", want, err)
		}
		var left int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&left); err != nil {
			t.Fatal(err)
		}
		if left != want {
			t.Fatalf("after down: %d, want %d", left, want)
		}
	}

	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"wallets", "transfers", "ledger_entries", "idempotency_records"} {
		var exists bool
		query := `SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1
		)`
		if err := pool.QueryRow(ctx, query, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Fatalf("table %s missing after re-migrate", table)
		}
	}
}

func envOrDefault() string {
	if value := os.Getenv("DATABASE_URL"); value != "" {
		return value
	}
	return "postgres://wallet:wallet@127.0.0.1:5433/wallet_transfer?sslmode=disable"
}
