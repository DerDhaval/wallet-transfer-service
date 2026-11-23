package repository_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"wallet-transfer-service/internal/domain"
	"wallet-transfer-service/internal/repository"
	"wallet-transfer-service/tests/testutil"
)

func TestMain(m *testing.M) {
	os.Exit(testutil.Run(m, "repository"))
}

func TestWalletRepository(t *testing.T) {
	pool := testutil.RequirePostgres(t)
	store := repository.New(pool)

	cases := []struct {
		name    string
		wallet  domain.Wallet
		wantErr bool
	}{
		{
			name: "insert active wallet",
			wallet: domain.Wallet{
				ID: "repo_wal_1", Balance: 100, OpeningBalance: 100, Currency: "INR", Status: domain.WalletActive,
			},
		},
		{
			name: "duplicate id",
			wallet: domain.Wallet{
				ID: "repo_wal_1", Balance: 1, OpeningBalance: 1, Currency: "INR", Status: domain.WalletActive,
			},
			wantErr: true,
		},
	}

	testutil.Clean(t, pool)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := tc.wallet
			err := store.InsertWallet(context.Background(), &w)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := store.GetWallet(context.Background(), w.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Balance != 100 || got.Currency != "INR" {
				t.Fatalf("got %+v", got)
			}
		})
	}

	t.Run("missing wallet", func(t *testing.T) {
		_, err := store.GetWallet(context.Background(), "missing")
		requireCode(t, err, "WALLET_NOT_FOUND")
	})

	t.Run("pool helpers", func(t *testing.T) {
		if store.Pool() == nil {
			t.Fatal("nil pool")
		}
		if err := store.Ping(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestTransferAndLedgerRepository(t *testing.T) {
	pool := testutil.RequirePostgres(t)
	testutil.Clean(t, pool)
	store := repository.New(pool)

	insertWallet(t, store, "repo_a", 200)
	insertWallet(t, store, "repo_b", 0)

	cases := []struct {
		name   string
		amount int64
		fail   bool
	}{
		{name: "processed transfer", amount: 40},
		{name: "failed transfer", amount: 10, fail: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := store.WithTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
				first, second := domain.OrderedWalletIDs("repo_a", "repo_b")
				if _, err := store.GetWalletForUpdate(ctx, tx, first); err != nil {
					return err
				}
				if _, err := store.GetWalletForUpdate(ctx, tx, second); err != nil {
					return err
				}
				transfer := domain.Transfer{
					ID:           domain.NewID("trf"),
					FromWalletID: "repo_a",
					ToWalletID:   "repo_b",
					Amount:       tc.amount,
					Currency:     "INR",
				}
				if err := store.InsertPendingTransfer(ctx, tx, &transfer); err != nil {
					return err
				}
				if tc.fail {
					return store.MarkTransferFailed(ctx, tx, &transfer, "INSUFFICIENT_FUNDS")
				}
				if err := store.DebitWallet(ctx, tx, "repo_a", tc.amount); err != nil {
					return err
				}
				if err := store.CreditWallet(ctx, tx, "repo_b", tc.amount); err != nil {
					return err
				}
				if err := store.InsertLedgerPair(ctx, tx, transfer); err != nil {
					return err
				}
				if err := store.MarkTransferProcessed(ctx, tx, &transfer); err != nil {
					return err
				}
				claimed, _, err := store.ClaimIdempotency(ctx, tx, "repo-key-"+tc.name, domain.RequestHash("repo_a", "repo_b", tc.amount))
				if err != nil {
					return err
				}
				if !claimed {
					t.Fatal("expected claim")
				}
				return store.CompleteIdempotency(ctx, tx, "repo-key-"+tc.name, transfer.ID, 201, `{"ok":true}`)
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}

	t.Run("get transfer and ledger page", func(t *testing.T) {
		entries, err := store.ListLedgerEntries(context.Background(), "repo_a", 10, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) == 0 {
			t.Fatal("expected ledger entries")
		}
		got, err := store.GetTransfer(context.Background(), entries[0].TransferID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != domain.StatusProcessed {
			t.Fatalf("status = %s", got.Status)
		}
	})

	t.Run("missing transfer", func(t *testing.T) {
		_, err := store.GetTransfer(context.Background(), "trf_missing")
		requireCode(t, err, "TRANSFER_NOT_FOUND")
	})

	t.Run("debit without funds fails inside tx", func(t *testing.T) {
		err := store.WithTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
			if _, err := store.GetWalletForUpdate(ctx, tx, "repo_b"); err != nil {
				return err
			}
			return store.DebitWallet(ctx, tx, "repo_b", 1_000_000)
		})
		if err == nil {
			t.Fatal("expected debit failure")
		}
	})

	t.Run("idempotency conflict path", func(t *testing.T) {
		err := store.WithTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
			claimed, existing, err := store.ClaimIdempotency(ctx, tx, "repo-key-processed transfer", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
			if err != nil {
				return err
			}
			if claimed {
				t.Fatal("should not claim existing key")
			}
			if existing.ResponseStatus != 201 {
				t.Fatalf("status = %d", existing.ResponseStatus)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("tx rollback on callback error", func(t *testing.T) {
		before, err := store.GetWallet(context.Background(), "repo_a")
		if err != nil {
			t.Fatal(err)
		}
		_ = store.WithTx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
			if _, err := store.GetWalletForUpdate(ctx, tx, "repo_a"); err != nil {
				return err
			}
			if err := store.DebitWallet(ctx, tx, "repo_a", 1); err != nil {
				return err
			}
			return context.Canceled
		})
		after, err := store.GetWallet(context.Background(), "repo_a")
		if err != nil {
			t.Fatal(err)
		}
		if after.Balance != before.Balance {
			t.Fatalf("rollback failed: %d -> %d", before.Balance, after.Balance)
		}
	})
}

func insertWallet(t *testing.T, store *repository.Store, id string, balance int64) {
	t.Helper()
	w := domain.Wallet{ID: id, Balance: balance, OpeningBalance: balance, Currency: "INR", Status: domain.WalletActive}
	if err := store.InsertWallet(context.Background(), &w); err != nil {
		t.Fatal(err)
	}
}

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	apiErr, ok := domain.AsError(err)
	if !ok || apiErr.Code != code {
		t.Fatalf("err = %v, want %s", err, code)
	}
}
