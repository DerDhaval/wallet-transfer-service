package service_test

import (
	"bytes"
	"context"
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
	svc, pool := setupService(t)
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
	testutil.DecodeJSON(t, result.Body, &body)
	if body.FromWalletID != "wallet_1" || body.ToWalletID != "wallet_2" || body.Amount != 100 || body.Currency != "INR" || body.Status != "PROCESSED" {
		t.Fatalf("body = %+v", body)
	}

	testutil.AssertBalance(t, pool, "wallet_1", 49_900)
	testutil.AssertBalance(t, pool, "wallet_2", 100)
	testutil.AssertTransferCount(t, pool, 1)
	testutil.AssertLedgerCount(t, pool, 2)
	testutil.AssertLedgerPair(t, pool, body.TransferID, "wallet_1", "wallet_2", 100)
	testutil.AssertBalanced(t, pool)

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
	svc, pool := setupService(t)
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
	testutil.AssertBalance(t, pool, "wallet_1", 460)
	testutil.AssertBalance(t, pool, "wallet_2", 60)
	testutil.AssertTransferCount(t, pool, 1)
	testutil.AssertLedgerCount(t, pool, 2)
	testutil.AssertBalanced(t, pool)
}

func TestIdempotencyKeyRejectsADifferentPayload(t *testing.T) {
	svc, pool := setupService(t)
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
	testutil.RequireCode(t, err, "IDEMPOTENCY_CONFLICT")

	replay, err := svc.CreateTransfer(context.Background(), original)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replayed || !bytes.Equal(first.Body, replay.Body) {
		t.Fatal("original result was not preserved after the conflict")
	}
	testutil.AssertBalance(t, pool, "wallet_1", 460)
	testutil.AssertTransferCount(t, pool, 1)
	testutil.AssertLedgerCount(t, pool, 2)
}

func TestConcurrentIdenticalRequestsCreateOneTransfer(t *testing.T) {
	svc, pool := setupService(t)
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
	testutil.AssertBalance(t, pool, "wallet_1", 75)
	testutil.AssertBalance(t, pool, "wallet_2", 25)
	testutil.AssertTransferCount(t, pool, 1)
	testutil.AssertLedgerCount(t, pool, 2)
	testutil.AssertBalanced(t, pool)
}

func TestConcurrentDebitsCannotOverdraw(t *testing.T) {
	svc, pool := setupService(t)
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
	source := testutil.BalanceOf(t, pool, "wallet_a")
	if source < 0 || source > 100 {
		t.Fatalf("source balance = %d", source)
	}
	moved := testutil.BalanceOf(t, pool, "wallet_b") + testutil.BalanceOf(t, pool, "wallet_c")
	if source+moved != 100 {
		t.Fatalf("money was created or lost: source %d destinations %d", source, moved)
	}
	if int64(processed*2) != testutil.Scalar(t, pool, `SELECT count(*) FROM ledger_entries`) {
		t.Fatal("ledger entries do not match processed transfers")
	}
	failedTransfers := testutil.Scalar(t, pool, `SELECT count(*) FROM transfers WHERE status = 'FAILED'`)
	if failedTransfers != int64(failed) {
		t.Fatalf("failed transfers = %d, want %d", failedTransfers, failed)
	}
	if testutil.Scalar(t, pool, `
		SELECT count(*) FROM transfers tf
		WHERE tf.status = 'FAILED' AND EXISTS (
			SELECT 1 FROM ledger_entries le WHERE le.transfer_id = tf.id
		)`) != 0 {
		t.Fatal("failed transfer has ledger entries")
	}
	testutil.AssertBalanced(t, pool)
}

func TestOppositeTransfersDoNotDeadlock(t *testing.T) {
	svc, pool := setupService(t)
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
	testutil.AssertBalance(t, pool, "wallet_a", 1_000)
	testutil.AssertBalance(t, pool, "wallet_b", 1_000)
	testutil.AssertTransferCount(t, pool, pairs*2)
	testutil.AssertLedgerCount(t, pool, pairs*4)
	testutil.AssertBalanced(t, pool)
}

func TestValidationAndMissingWalletDoNotConsumeTheKey(t *testing.T) {
	svc, pool := setupService(t)

	_, err := svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
		IdempotencyKey: "key-1", FromWalletID: "wallet_1", ToWalletID: "wallet_1", Amount: 10,
	})
	testutil.RequireCode(t, err, "SAME_WALLET")

	_, err = svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
		IdempotencyKey: "key-1", FromWalletID: "wallet_1", ToWalletID: "wallet_2", Amount: 0,
	})
	testutil.RequireCode(t, err, "INVALID_AMOUNT")

	_, err = svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
		IdempotencyKey: "key-1", FromWalletID: "wallet_1", ToWalletID: "wallet_2", Amount: -3,
	})
	testutil.RequireCode(t, err, "INVALID_AMOUNT")

	_, err = svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
		IdempotencyKey: "key-1", FromWalletID: "missing", ToWalletID: "wallet_2", Amount: 10,
	})
	testutil.RequireCode(t, err, "WALLET_NOT_FOUND")

	createWallet(t, svc, "wallet_1", "INR", 30)
	_, err = svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
		IdempotencyKey: "key-1", FromWalletID: "wallet_1", ToWalletID: "missing", Amount: 10,
	})
	testutil.RequireCode(t, err, "WALLET_NOT_FOUND")

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
	testutil.AssertBalance(t, pool, "wallet_1", 20)
	testutil.AssertTransferCount(t, pool, 1)
	testutil.AssertLedgerCount(t, pool, 2)
}

func TestInsufficientFundsIsFinalForTheKeyAndLeavesNoLedger(t *testing.T) {
	svc, pool := setupService(t)
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
	testutil.AssertBalance(t, pool, "wallet_1", 100)
	testutil.AssertBalance(t, pool, "wallet_2", 5)
	testutil.AssertLedgerCount(t, pool, 0)
	if testutil.Scalar(t, pool, `SELECT count(*) FROM transfers WHERE status = 'FAILED' AND failure_reason = 'INSUFFICIENT_FUNDS'`) != 1 {
		t.Fatal("failed transfer was not recorded once")
	}

	ok, err := svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
		IdempotencyKey: "smaller", FromWalletID: "wallet_1", ToWalletID: "wallet_2", Amount: 100,
	})
	if err != nil || ok.StatusCode != 201 {
		t.Fatalf("new key should succeed after a failed key: %v %+v", err, ok)
	}
	testutil.AssertBalance(t, pool, "wallet_1", 0)
	testutil.AssertBalance(t, pool, "wallet_2", 105)
	testutil.AssertBalanced(t, pool)
}

func TestInactiveWalletAndCurrencyMismatchDoNotMoveMoney(t *testing.T) {
	svc, pool := setupService(t)
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
	testutil.AssertBalance(t, pool, "wallet_1", 80)
	testutil.AssertBalance(t, pool, "wallet_2", 10)
	testutil.AssertBalance(t, pool, "wallet_3", 10)
	testutil.AssertLedgerCount(t, pool, 0)
	testutil.AssertBalanced(t, pool)
}

func TestTransactionFailureRollsBackAndRetrySucceeds(t *testing.T) {
	pool := testutil.FreshPool(t)
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
	testutil.AssertBalance(t, pool, "wallet_1", 70)
	testutil.AssertBalance(t, pool, "wallet_2", 5)
	testutil.AssertTransferCount(t, pool, 0)
	testutil.AssertLedgerCount(t, pool, 0)
	if testutil.Scalar(t, pool, `SELECT count(*) FROM idempotency_records`) != 0 {
		t.Fatal("rolled-back request consumed the idempotency key")
	}

	recovered := service.New(repository.New(pool))
	result, err := recovered.CreateTransfer(context.Background(), cmd)
	if err != nil || result.StatusCode != 201 || result.Replayed {
		t.Fatalf("retry after rollback: %v %+v", err, result)
	}
	testutil.AssertBalance(t, pool, "wallet_1", 55)
	testutil.AssertBalance(t, pool, "wallet_2", 20)
	testutil.AssertLedgerCount(t, pool, 2)
	testutil.AssertBalanced(t, pool)
}

func TestAmountOverflowDoesNotCredit(t *testing.T) {
	svc, pool := setupService(t)
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
	testutil.AssertBalance(t, pool, "wallet_1", 100)
	testutil.AssertBalance(t, pool, "wallet_max", math.MaxInt64-5)
	testutil.AssertLedgerCount(t, pool, 0)
}

func TestDatabaseGuardsRejectPartialFinancialWrites(t *testing.T) {
	svc, pool := setupService(t)
	createWallet(t, svc, "wallet_1", "INR", 40)
	createWallet(t, svc, "wallet_2", "INR", 0)
	result, err := svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
		IdempotencyKey: "guard", FromWalletID: "wallet_1", ToWalletID: "wallet_2", Amount: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	transferID := testutil.TransferIDFromBody(t, result.Body)

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
	testutil.AssertBalance(t, pool, "wallet_1", 30)
	testutil.AssertBalance(t, pool, "wallet_2", 10)
	testutil.AssertBalanced(t, pool)
}

func setupService(t *testing.T) (*service.Service, *pgxpool.Pool) {
	t.Helper()
	pool := testutil.FreshPool(t)
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
