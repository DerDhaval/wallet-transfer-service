package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"wallet-transfer-service/internal/domain"
	"wallet-transfer-service/internal/repository"
	"wallet-transfer-service/internal/service"
	"wallet-transfer-service/tests/testutil"
)

func TestMain(m *testing.M) {
	os.Exit(testutil.Run(m, "service"))
}

func TestTransferSucceedsAndPostsBothLedgerEntries(t *testing.T) {
	svc, pool := setup(t)
	createWallet(t, svc, "wallet_1", "INR", 50_000)
	createWallet(t, svc, "wallet_2", "INR", 0)

	result, err := svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
		IdempotencyKey: "abc123",
		FromWalletID:   "wallet_1",
		ToWalletID:     "wallet_2",
		Amount:         100,
	})
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if result.StatusCode != 201 || result.Replayed {
		t.Fatalf("status = %d replayed = %v", result.StatusCode, result.Replayed)
	}

	var body struct {
		TransferID   string `json:"transferId"`
		FromWalletID string `json:"fromWalletId"`
		ToWalletID   string `json:"toWalletId"`
		Amount       int64  `json:"amount"`
		Currency     string `json:"currency"`
		Status       string `json:"status"`
	}
	decode(t, result.Body, &body)
	if body.FromWalletID != "wallet_1" || body.ToWalletID != "wallet_2" || body.Amount != 100 || body.Currency != "INR" || body.Status != "PROCESSED" {
		t.Fatalf("body = %+v", body)
	}

	assertBalance(t, pool, "wallet_1", 49_900)
	assertBalance(t, pool, "wallet_2", 100)
	assertTransferCount(t, pool, 1)
	assertLedgerCount(t, pool, 2)
	assertLedgerPair(t, pool, body.TransferID, "wallet_1", "wallet_2", 100)
	assertBalanced(t, pool)

	stored, err := svc.GetTransfer(context.Background(), body.TransferID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != domain.StatusProcessed || stored.Amount != 100 {
		t.Fatalf("stored = %+v", stored)
	}

	sourceEntries, err := svc.ListWalletEntries(context.Background(), "wallet_1", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(sourceEntries) != 1 || sourceEntries[0].EntryType != domain.EntryDebit || sourceEntries[0].Amount != 100 {
		t.Fatalf("source entries = %+v", sourceEntries)
	}
	destinationEntries, err := svc.ListWalletEntries(context.Background(), "wallet_2", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(destinationEntries) != 1 || destinationEntries[0].EntryType != domain.EntryCredit {
		t.Fatalf("destination entries = %+v", destinationEntries)
	}
}

func TestIdempotencyReplayDoesNotMoveMoneyAgain(t *testing.T) {
	svc, pool := setup(t)
	createWallet(t, svc, "wallet_1", "INR", 500)
	createWallet(t, svc, "wallet_2", "INR", 20)

	cmd := service.CreateTransferCommand{
		IdempotencyKey: "same-key",
		FromWalletID:   "wallet_1",
		ToWalletID:     "wallet_2",
		Amount:         40,
	}
	first, err := svc.CreateTransfer(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.CreateTransfer(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	third, err := svc.CreateTransfer(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed || !third.Replayed {
		t.Fatal("retries were executed again")
	}
	if first.StatusCode != 201 || !bytes.Equal(first.Body, second.Body) || !bytes.Equal(first.Body, third.Body) {
		t.Fatalf("replay changed the response\nfirst=%s\nsecond=%s", first.Body, second.Body)
	}
	assertBalance(t, pool, "wallet_1", 460)
	assertBalance(t, pool, "wallet_2", 60)
	assertTransferCount(t, pool, 1)
	assertLedgerCount(t, pool, 2)
	assertBalanced(t, pool)
}

func TestIdempotencyKeyRejectsADifferentPayload(t *testing.T) {
	svc, pool := setup(t)
	createWallet(t, svc, "wallet_1", "INR", 500)
	createWallet(t, svc, "wallet_2", "INR", 0)

	original := service.CreateTransferCommand{IdempotencyKey: "same-key", FromWalletID: "wallet_1", ToWalletID: "wallet_2", Amount: 40}
	first, err := svc.CreateTransfer(context.Background(), original)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
		IdempotencyKey: "same-key",
		FromWalletID:   "wallet_1",
		ToWalletID:     "wallet_2",
		Amount:         41,
	})
	requireCode(t, err, "IDEMPOTENCY_CONFLICT")

	replay, err := svc.CreateTransfer(context.Background(), original)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replayed || !bytes.Equal(first.Body, replay.Body) {
		t.Fatal("original result was not preserved after the conflict")
	}
	assertBalance(t, pool, "wallet_1", 460)
	assertTransferCount(t, pool, 1)
	assertLedgerCount(t, pool, 2)
}

func TestConcurrentIdenticalRequestsCreateOneTransfer(t *testing.T) {
	svc, pool := setup(t)
	createWallet(t, svc, "wallet_1", "INR", 100)
	createWallet(t, svc, "wallet_2", "INR", 0)

	const n = 20
	cmd := service.CreateTransferCommand{IdempotencyKey: "race-key", FromWalletID: "wallet_1", ToWalletID: "wallet_2", Amount: 25}
	results := make([]service.Result, n)
	errs := make([]error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = svc.CreateTransfer(context.Background(), cmd)
		}(i)
	}
	close(start)
	wg.Wait()

	var body []byte
	replays := 0
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("request %d: %v", i, errs[i])
		}
		if results[i].StatusCode != 201 {
			t.Fatalf("request %d status = %d body = %s", i, results[i].StatusCode, results[i].Body)
		}
		if body == nil {
			body = results[i].Body
		} else if !bytes.Equal(body, results[i].Body) {
			t.Fatalf("responses diverged\n%s\n%s", body, results[i].Body)
		}
		if results[i].Replayed {
			replays++
		}
	}
	if replays != n-1 {
		t.Fatalf("replays = %d, want %d", replays, n-1)
	}
	assertBalance(t, pool, "wallet_1", 75)
	assertBalance(t, pool, "wallet_2", 25)
	assertTransferCount(t, pool, 1)
	assertLedgerCount(t, pool, 2)
	assertBalanced(t, pool)
}

func TestConcurrentDebitsCannotOverdraw(t *testing.T) {
	svc, pool := setup(t)
	createWallet(t, svc, "wallet_a", "INR", 100)
	createWallet(t, svc, "wallet_b", "INR", 0)
	createWallet(t, svc, "wallet_c", "INR", 0)

	commands := []service.CreateTransferCommand{
		{IdempotencyKey: "t-b", FromWalletID: "wallet_a", ToWalletID: "wallet_b", Amount: 80},
		{IdempotencyKey: "t-c", FromWalletID: "wallet_a", ToWalletID: "wallet_c", Amount: 50},
	}
	// Add more contenders so the result does not depend on two goroutines getting lucky.
	for i := 0; i < 8; i++ {
		commands = append(commands, service.CreateTransferCommand{
			IdempotencyKey: fmt.Sprintf("extra-%d", i),
			FromWalletID:   "wallet_a",
			ToWalletID:     "wallet_b",
			Amount:         30,
		})
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	type outcome struct {
		status int
		err    error
	}
	out := make([]outcome, len(commands))
	wg.Add(len(commands))
	for i := range commands {
		go func(i int) {
			defer wg.Done()
			<-start
			result, err := svc.CreateTransfer(context.Background(), commands[i])
			out[i] = outcome{status: result.StatusCode, err: err}
		}(i)
	}
	close(start)
	wg.Wait()

	processed := 0
	failed := 0
	for i, item := range out {
		if item.err != nil {
			t.Fatalf("request %d: %v", i, item.err)
		}
		switch item.status {
		case 201:
			processed++
		case 422:
			failed++
		default:
			t.Fatalf("request %d status = %d", i, item.status)
		}
	}
	if processed == 0 || failed == 0 {
		t.Fatalf("processed = %d failed = %d, want both", processed, failed)
	}
	source := balanceOf(t, pool, "wallet_a")
	if source < 0 || source > 100 {
		t.Fatalf("source balance = %d", source)
	}
	moved := balanceOf(t, pool, "wallet_b") + balanceOf(t, pool, "wallet_c")
	if source+moved != 100 {
		t.Fatalf("money was created or lost: source %d destinations %d", source, moved)
	}
	if int64(processed*2) != scalar(t, pool, `SELECT count(*) FROM ledger_entries`) {
		t.Fatal("ledger entries do not match processed transfers")
	}
	failedTransfers := scalar(t, pool, `SELECT count(*) FROM transfers WHERE status = 'FAILED'`)
	if failedTransfers != int64(failed) {
		t.Fatalf("failed transfers = %d, want %d", failedTransfers, failed)
	}
	if scalar(t, pool, `
		SELECT count(*) FROM transfers tf
		WHERE tf.status = 'FAILED' AND EXISTS (
			SELECT 1 FROM ledger_entries le WHERE le.transfer_id = tf.id
		)`) != 0 {
		t.Fatal("failed transfer has ledger entries")
	}
	assertBalanced(t, pool)
}

func TestOppositeTransfersDoNotDeadlock(t *testing.T) {
	svc, pool := setup(t)
	createWallet(t, svc, "wallet_a", "INR", 1_000)
	createWallet(t, svc, "wallet_b", "INR", 1_000)

	const pairs = 15
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, pairs*2)
	wg.Add(pairs * 2)
	for i := 0; i < pairs; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
				IdempotencyKey: fmt.Sprintf("ab-%d", i),
				FromWalletID:   "wallet_a",
				ToWalletID:     "wallet_b",
				Amount:         10,
			})
			errs <- err
		}(i)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
				IdempotencyKey: fmt.Sprintf("ba-%d", i),
				FromWalletID:   "wallet_b",
				ToWalletID:     "wallet_a",
				Amount:         10,
			})
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	assertBalance(t, pool, "wallet_a", 1_000)
	assertBalance(t, pool, "wallet_b", 1_000)
	assertTransferCount(t, pool, pairs*2)
	assertLedgerCount(t, pool, pairs*4)
	assertBalanced(t, pool)
}

func TestValidationAndMissingWalletDoNotConsumeTheKey(t *testing.T) {
	svc, pool := setup(t)

	_, err := svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
		IdempotencyKey: "key-1", FromWalletID: "wallet_1", ToWalletID: "wallet_1", Amount: 10,
	})
	requireCode(t, err, "SAME_WALLET")

	_, err = svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
		IdempotencyKey: "key-1", FromWalletID: "wallet_1", ToWalletID: "wallet_2", Amount: 0,
	})
	requireCode(t, err, "INVALID_AMOUNT")

	_, err = svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
		IdempotencyKey: "key-1", FromWalletID: "wallet_1", ToWalletID: "wallet_2", Amount: -3,
	})
	requireCode(t, err, "INVALID_AMOUNT")

	_, err = svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
		IdempotencyKey: "key-1", FromWalletID: "missing", ToWalletID: "wallet_2", Amount: 10,
	})
	requireCode(t, err, "WALLET_NOT_FOUND")

	createWallet(t, svc, "wallet_1", "INR", 30)
	_, err = svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
		IdempotencyKey: "key-1", FromWalletID: "wallet_1", ToWalletID: "missing", Amount: 10,
	})
	requireCode(t, err, "WALLET_NOT_FOUND")

	createWallet(t, svc, "wallet_2", "INR", 0)
	result, err := svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
		IdempotencyKey: "key-1", FromWalletID: "wallet_1", ToWalletID: "wallet_2", Amount: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != 201 || result.Replayed {
		t.Fatalf("key was consumed by an earlier rejection: %+v", result)
	}
	assertBalance(t, pool, "wallet_1", 20)
	assertTransferCount(t, pool, 1)
	assertLedgerCount(t, pool, 2)
}

func TestInsufficientFundsIsFinalForTheKeyAndLeavesNoLedger(t *testing.T) {
	svc, pool := setup(t)
	createWallet(t, svc, "wallet_1", "INR", 100)
	createWallet(t, svc, "wallet_2", "INR", 5)

	cmd := service.CreateTransferCommand{IdempotencyKey: "too-big", FromWalletID: "wallet_1", ToWalletID: "wallet_2", Amount: 101}
	first, err := svc.CreateTransfer(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if first.StatusCode != 422 {
		t.Fatalf("status = %d body = %s", first.StatusCode, first.Body)
	}
	second, err := svc.CreateTransfer(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed || !bytes.Equal(first.Body, second.Body) {
		t.Fatal("insufficient funds was executed again")
	}
	assertBalance(t, pool, "wallet_1", 100)
	assertBalance(t, pool, "wallet_2", 5)
	assertLedgerCount(t, pool, 0)
	if scalar(t, pool, `SELECT count(*) FROM transfers WHERE status = 'FAILED' AND failure_reason = 'INSUFFICIENT_FUNDS'`) != 1 {
		t.Fatal("failed transfer was not recorded once")
	}

	ok, err := svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
		IdempotencyKey: "smaller", FromWalletID: "wallet_1", ToWalletID: "wallet_2", Amount: 100,
	})
	if err != nil || ok.StatusCode != 201 {
		t.Fatalf("new key should succeed after a failed key: %v %+v", err, ok)
	}
	assertBalance(t, pool, "wallet_1", 0)
	assertBalance(t, pool, "wallet_2", 105)
	assertBalanced(t, pool)
}

func TestInactiveWalletAndCurrencyMismatchDoNotMoveMoney(t *testing.T) {
	svc, pool := setup(t)
	createWallet(t, svc, "wallet_1", "INR", 80)
	createWallet(t, svc, "wallet_2", "INR", 10)
	createWallet(t, svc, "wallet_3", "USD", 10)
	if _, err := pool.Exec(context.Background(), `UPDATE wallets SET status = 'FROZEN' WHERE id = 'wallet_2'`); err != nil {
		t.Fatal(err)
	}

	frozen, err := svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
		IdempotencyKey: "frozen", FromWalletID: "wallet_1", ToWalletID: "wallet_2", Amount: 5,
	})
	if err != nil || frozen.StatusCode != 422 {
		t.Fatalf("frozen: %v %+v", err, frozen)
	}
	mismatch, err := svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
		IdempotencyKey: "fx", FromWalletID: "wallet_1", ToWalletID: "wallet_3", Amount: 5,
	})
	if err != nil || mismatch.StatusCode != 422 {
		t.Fatalf("currency: %v %+v", err, mismatch)
	}
	assertBalance(t, pool, "wallet_1", 80)
	assertBalance(t, pool, "wallet_2", 10)
	assertBalance(t, pool, "wallet_3", 10)
	assertLedgerCount(t, pool, 0)
	assertBalanced(t, pool)
}

func TestTransactionFailureRollsBackAndRetrySucceeds(t *testing.T) {
	pool := testutil.RequirePostgres(t)
	testutil.Clean(t, pool)
	crashed := service.New(repository.New(pool), service.WithBeforeCommit(func(context.Context) error {
		return errors.New("database unavailable before commit")
	}))
	createWallet(t, crashed, "wallet_1", "INR", 70)
	createWallet(t, crashed, "wallet_2", "INR", 5)

	cmd := service.CreateTransferCommand{IdempotencyKey: "retry-me", FromWalletID: "wallet_1", ToWalletID: "wallet_2", Amount: 15}
	_, err := crashed.CreateTransfer(context.Background(), cmd)
	if err == nil {
		t.Fatal("expected the pre-commit failure")
	}
	if apiErr, ok := domain.AsError(err); ok && apiErr.Status < 500 {
		t.Fatalf("rollback was reported as a client error: %v", err)
	}
	assertBalance(t, pool, "wallet_1", 70)
	assertBalance(t, pool, "wallet_2", 5)
	assertTransferCount(t, pool, 0)
	assertLedgerCount(t, pool, 0)
	if scalar(t, pool, `SELECT count(*) FROM idempotency_records`) != 0 {
		t.Fatal("rolled-back request consumed the idempotency key")
	}

	recovered := service.New(repository.New(pool))
	result, err := recovered.CreateTransfer(context.Background(), cmd)
	if err != nil || result.StatusCode != 201 || result.Replayed {
		t.Fatalf("retry after rollback: %v %+v", err, result)
	}
	assertBalance(t, pool, "wallet_1", 55)
	assertBalance(t, pool, "wallet_2", 20)
	assertLedgerCount(t, pool, 2)
	assertBalanced(t, pool)
}

func TestAmountOverflowDoesNotCredit(t *testing.T) {
	svc, pool := setup(t)
	createWallet(t, svc, "wallet_1", "INR", 100)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO wallets (id, balance, opening_balance, currency, status)
		VALUES ('wallet_max', $1, $1, 'INR', 'ACTIVE')`, math.MaxInt64-5); err != nil {
		t.Fatal(err)
	}
	result, err := svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
		IdempotencyKey: "overflow", FromWalletID: "wallet_1", ToWalletID: "wallet_max", Amount: 10,
	})
	if err != nil || result.StatusCode != 422 {
		t.Fatalf("overflow: %v %+v", err, result)
	}
	assertBalance(t, pool, "wallet_1", 100)
	assertBalance(t, pool, "wallet_max", math.MaxInt64-5)
	assertLedgerCount(t, pool, 0)
}

func TestDatabaseGuardsRejectPartialFinancialWrites(t *testing.T) {
	svc, pool := setup(t)
	createWallet(t, svc, "wallet_1", "INR", 40)
	createWallet(t, svc, "wallet_2", "INR", 0)
	result, err := svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
		IdempotencyKey: "guard", FromWalletID: "wallet_1", ToWalletID: "wallet_2", Amount: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	transferID := transferID(t, result.Body)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO transfers (id, from_wallet_id, to_wallet_id, amount, currency, status)
		VALUES ('trf_pending', 'wallet_1', 'wallet_2', 1, 'INR', 'PENDING')`)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("PENDING")) {
		t.Fatalf("pending commit err = %v", err)
	}

	_, err = pool.Exec(context.Background(), `
		UPDATE transfers SET status = 'FAILED', failure_reason = 'NO' WHERE id = $1`, transferID)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("terminal")) {
		t.Fatalf("terminal update err = %v", err)
	}

	_, err = pool.Exec(context.Background(), `UPDATE ledger_entries SET amount = amount + 1 WHERE transfer_id = $1`, transferID)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("immutable")) {
		t.Fatalf("ledger update err = %v", err)
	}
	_, err = pool.Exec(context.Background(), `DELETE FROM ledger_entries WHERE transfer_id = $1`, transferID)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("immutable")) {
		t.Fatalf("ledger delete err = %v", err)
	}

	_, err = pool.Exec(context.Background(), `UPDATE wallets SET balance = -1 WHERE id = 'wallet_1'`)
	if err == nil {
		t.Fatal("negative balance was stored")
	}

	_, err = pool.Exec(context.Background(), `UPDATE wallets SET balance = balance + 1 WHERE id = 'wallet_1'`)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("inconsistent")) {
		t.Fatalf("drift err = %v", err)
	}
	assertBalance(t, pool, "wallet_1", 30)
	assertBalance(t, pool, "wallet_2", 10)
	assertBalanced(t, pool)
}

func setup(t *testing.T) (*service.Service, *pgxpool.Pool) {
	t.Helper()
	pool := testutil.RequirePostgres(t)
	testutil.Clean(t, pool)
	return service.New(repository.New(pool)), pool
}

func createWallet(t *testing.T, svc *service.Service, id, currency string, balance int64) {
	t.Helper()
	_, err := svc.CreateWallet(context.Background(), service.CreateWalletCommand{
		ID:             id,
		Currency:       currency,
		InitialBalance: balance,
	})
	if err != nil {
		t.Fatalf("create wallet %s: %v", id, err)
	}
}

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	apiErr, ok := domain.AsError(err)
	if !ok {
		t.Fatalf("got %v, want %s", err, code)
	}
	if apiErr.Code != code {
		t.Fatalf("code = %s, want %s", apiErr.Code, code)
	}
}

func decode(t *testing.T, body []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(body, dst); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
}

func transferID(t *testing.T, body []byte) string {
	t.Helper()
	var payload struct {
		TransferID string `json:"transferId"`
	}
	decode(t, body, &payload)
	if payload.TransferID == "" {
		t.Fatalf("no transfer id in %s", body)
	}
	return payload.TransferID
}

func assertBalance(t *testing.T, pool *pgxpool.Pool, id string, want int64) {
	t.Helper()
	if got := balanceOf(t, pool, id); got != want {
		t.Fatalf("balance %s = %d, want %d", id, got, want)
	}
}

func balanceOf(t *testing.T, pool *pgxpool.Pool, id string) int64 {
	t.Helper()
	var balance int64
	if err := pool.QueryRow(context.Background(), `SELECT balance FROM wallets WHERE id = $1`, id).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	return balance
}

func assertTransferCount(t *testing.T, pool *pgxpool.Pool, want int64) {
	t.Helper()
	if got := scalar(t, pool, `SELECT count(*) FROM transfers`); got != want {
		t.Fatalf("transfers = %d, want %d", got, want)
	}
}

func assertLedgerCount(t *testing.T, pool *pgxpool.Pool, want int64) {
	t.Helper()
	if got := scalar(t, pool, `SELECT count(*) FROM ledger_entries`); got != want {
		t.Fatalf("ledger entries = %d, want %d", got, want)
	}
}

func assertLedgerPair(t *testing.T, pool *pgxpool.Pool, transferID, from, to string, amount int64) {
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
	if len(got) != 2 || got[0].kind != "CREDIT" || got[0].wallet != to || got[0].amount != amount || got[1].kind != "DEBIT" || got[1].wallet != from || got[1].amount != amount {
		t.Fatalf("ledger pair = %+v", got)
	}
}

func assertBalanced(t *testing.T, pool *pgxpool.Pool) {
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

func scalar(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int64 {
	t.Helper()
	var value int64
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}
