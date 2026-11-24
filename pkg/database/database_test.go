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
	const migrationCount = 5 // four tables + cross-currency transfers
	if ups != migrationCount {
		t.Fatalf("applied = %d, want %d migrations", ups, migrationCount)
	}

	for want := migrationCount - 1; want >= 0; want-- {
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

// An existing database already has same-currency transfers. Upgrading must backfill them.
func TestMigrateUpgradesExistingTransfers(t *testing.T) {
	pool := testutil.RequirePostgres(t)
	ctx := context.Background()

	if err := database.MigrateDown(ctx, pool); err != nil { // back to 000004
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`INSERT INTO wallets (id, balance, opening_balance, currency) VALUES ('old_a', 70, 100, 'INR'), ('old_b', 30, 0, 'INR')`,
		`INSERT INTO transfers (id, from_wallet_id, to_wallet_id, amount, currency, status) VALUES ('old_t1', 'old_a', 'old_b', 30, 'INR', 'PENDING')`,
		`INSERT INTO ledger_entries (id, wallet_id, transfer_id, entry_type, amount) VALUES ('old_l1', 'old_a', 'old_t1', 'DEBIT', 30), ('old_l2', 'old_b', 'old_t1', 'CREDIT', 30)`,
		`UPDATE transfers SET status = 'PROCESSED' WHERE id = 'old_t1'`,
		`INSERT INTO transfers (id, from_wallet_id, to_wallet_id, amount, currency, status, failure_reason) VALUES ('old_t2', 'old_a', 'old_b', 500, 'INR', 'PENDING', NULL)`,
		`UPDATE transfers SET status = 'FAILED', failure_reason = 'INSUFFICIENT_FUNDS' WHERE id = 'old_t2'`,
	} {
		if _, err := tx.Exec(ctx, stmt); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("upgrade with existing data: %v", err)
	}
	var toCurrency, rate string
	var toAmount int64
	err = pool.QueryRow(ctx, `SELECT to_currency, to_amount, exchange_rate FROM transfers WHERE id = 'old_t1'`).Scan(&toCurrency, &toAmount, &rate)
	if err != nil || toCurrency != "INR" || toAmount != 30 || rate != "1" {
		t.Fatalf("backfill = %q %d %q err = %v", toCurrency, toAmount, rate, err)
	}
	// The triggers must be back on after the backfill.
	if _, err := pool.Exec(ctx, `UPDATE transfers SET amount = 31 WHERE id = 'old_t1'`); err == nil {
		t.Fatal("transfer immutability trigger is not enabled after upgrade")
	}
}

func envOrDefault() string {
	if value := os.Getenv("DATABASE_URL"); value != "" {
		return value
	}
	return "postgres://wallet:wallet@127.0.0.1:5433/wallet_transfer?sslmode=disable"
}
