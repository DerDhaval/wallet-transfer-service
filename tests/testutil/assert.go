package testutil

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"wallet-transfer-service/internal/domain"
)

// FreshPool returns a cleaned migrated pool for one test.
func FreshPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := RequirePostgres(t)
	Clean(t, pool)
	return pool
}

// RequireCode asserts err is a domain error with the given code.
func RequireCode(t *testing.T, err error, code string) {
	t.Helper()
	apiErr, ok := domain.AsError(err)
	if !ok {
		t.Fatalf("got %v, want domain error %s", err, code)
	}
	if apiErr.Code != code {
		t.Fatalf("code = %s, want %s", apiErr.Code, code)
	}
}

// DecodeJSON unmarshals body into dst or fails the test.
func DecodeJSON(t *testing.T, body []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(body, dst); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
}

// TransferIDFromBody reads transferId from a JSON response body.
func TransferIDFromBody(t *testing.T, body []byte) string {
	t.Helper()
	var payload struct {
		TransferID string `json:"transferId"`
	}
	DecodeJSON(t, body, &payload)
	if payload.TransferID == "" {
		t.Fatalf("no transfer id in %s", body)
	}
	return payload.TransferID
}

// InsertWallet inserts a wallet row directly (for repository / setup helpers).
func InsertWallet(t *testing.T, pool *pgxpool.Pool, id, currency string, balance int64) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO wallets (id, balance, opening_balance, currency, status)
		VALUES ($1, $2, $2, $3, 'ACTIVE')`, id, balance, currency)
	if err != nil {
		t.Fatalf("insert wallet %s: %v", id, err)
	}
}

// BalanceOf returns the current wallet balance.
func BalanceOf(t *testing.T, pool *pgxpool.Pool, id string) int64 {
	t.Helper()
	var balance int64
	if err := pool.QueryRow(context.Background(), `SELECT balance FROM wallets WHERE id = $1`, id).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	return balance
}

// AssertBalance checks one wallet balance.
func AssertBalance(t *testing.T, pool *pgxpool.Pool, id string, want int64) {
	t.Helper()
	if got := BalanceOf(t, pool, id); got != want {
		t.Fatalf("balance %s = %d, want %d", id, got, want)
	}
}

// AssertTransferCount checks how many transfer rows exist.
func AssertTransferCount(t *testing.T, pool *pgxpool.Pool, want int64) {
	t.Helper()
	if got := Scalar(t, pool, `SELECT count(*) FROM transfers`); got != want {
		t.Fatalf("transfers = %d, want %d", got, want)
	}
}

// AssertLedgerCount checks how many ledger rows exist.
func AssertLedgerCount(t *testing.T, pool *pgxpool.Pool, want int64) {
	t.Helper()
	if got := Scalar(t, pool, `SELECT count(*) FROM ledger_entries`); got != want {
		t.Fatalf("ledger entries = %d, want %d", got, want)
	}
}

// AssertLedgerPair checks the debit/credit pair for a same-currency transfer.
func AssertLedgerPair(t *testing.T, pool *pgxpool.Pool, transferID, from, to string, amount int64) {
	t.Helper()
	AssertLedgerAmounts(t, pool, transferID, from, amount, to, amount)
}

// AssertLedgerAmounts checks the debit (in the source currency) and the credit
// (in the destination currency) for one transfer.
func AssertLedgerAmounts(t *testing.T, pool *pgxpool.Pool, transferID, from string, debit int64, to string, credit int64) {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT wallet_id, entry_type, amount
		FROM ledger_entries
		WHERE transfer_id = $1
		ORDER BY entry_type`, transferID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	type pair struct {
		wallet string
		kind   string
		amount int64
	}
	var got []pair
	for rows.Next() {
		var item pair
		if err := rows.Scan(&item.wallet, &item.kind, &item.amount); err != nil {
			t.Fatal(err)
		}
		got = append(got, item)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 ||
		got[0].kind != "CREDIT" || got[0].wallet != to || got[0].amount != credit ||
		got[1].kind != "DEBIT" || got[1].wallet != from || got[1].amount != debit {
		t.Fatalf("ledger pair = %+v", got)
	}
}

// AssertBalanced checks balance = opening_balance + credits - debits for every wallet.
func AssertBalanced(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT w.id,
		       w.balance,
		       w.opening_balance + COALESCE(SUM(
		           CASE
		               WHEN l.entry_type = 'CREDIT' THEN l.amount
		               WHEN l.entry_type = 'DEBIT' THEN -l.amount
		               ELSE 0
		           END
		       ), 0) AS derived
		FROM wallets w
		LEFT JOIN ledger_entries l ON l.wallet_id = w.id
		GROUP BY w.id, w.balance, w.opening_balance`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var balance, derived int64
		if err := rows.Scan(&id, &balance, &derived); err != nil {
			t.Fatal(err)
		}
		if balance != derived {
			t.Fatalf("wallet %s balance %d derived %d", id, balance, derived)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

// Scalar runs a single-column integer query.
func Scalar(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int64 {
	t.Helper()
	var value int64
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}
