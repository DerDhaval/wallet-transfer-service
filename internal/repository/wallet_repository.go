package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"wallet-transfer-service/internal/domain"
)

const walletColumns = `id, balance, opening_balance, currency, status, created_at, updated_at`

func scanWallet(row pgx.Row) (domain.Wallet, error) {
	var w domain.Wallet
	var status string
	if err := row.Scan(&w.ID, &w.Balance, &w.OpeningBalance, &w.Currency, &status, &w.CreatedAt, &w.UpdatedAt); err != nil {
		return domain.Wallet{}, err
	}
	w.Status = domain.WalletStatus(status)
	w.CreatedAt = utc(w.CreatedAt)
	w.UpdatedAt = utc(w.UpdatedAt)
	return w, nil
}

func (s *Store) InsertWallet(ctx context.Context, w *domain.Wallet) error {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO wallets (id, balance, opening_balance, currency, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+walletColumns,
		w.ID, w.Balance, w.OpeningBalance, w.Currency, string(w.Status),
	)
	scanned, err := scanWallet(row)
	if err != nil {
		return fmt.Errorf("insert wallet: %w", err)
	}
	*w = scanned
	return nil
}

func (s *Store) GetWallet(ctx context.Context, id string) (domain.Wallet, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+walletColumns+` FROM wallets WHERE id = $1`, id)
	w, err := scanWallet(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Wallet{}, domain.ErrWalletNotFound.WithMessage("wallet " + id + " was not found")
	}
	if err != nil {
		return domain.Wallet{}, fmt.Errorf("get wallet: %w", err)
	}
	return w, nil
}

// GetWalletForUpdate locks the wallet row. Callers must lock multiple wallets via domain.OrderedWalletIDs.
func (s *Store) GetWalletForUpdate(ctx context.Context, tx pgx.Tx, id string) (domain.Wallet, error) {
	row := tx.QueryRow(ctx, `SELECT `+walletColumns+` FROM wallets WHERE id = $1 FOR UPDATE`, id)
	w, err := scanWallet(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Wallet{}, domain.ErrWalletNotFound.WithMessage("wallet " + id + " was not found")
	}
	if err != nil {
		return domain.Wallet{}, fmt.Errorf("lock wallet: %w", err)
	}
	return w, nil
}

func (s *Store) DebitWallet(ctx context.Context, tx pgx.Tx, id string, amount int64) error {
	tag, err := tx.Exec(ctx, `
		UPDATE wallets
		SET balance = balance - $1, updated_at = now()
		WHERE id = $2 AND balance >= $1 AND status = 'ACTIVE'`,
		amount, id,
	)
	if err != nil {
		return fmt.Errorf("debit wallet: %w", err)
	}
	return mustAffectOne(tag, "debit wallet")
}

func (s *Store) CreditWallet(ctx context.Context, tx pgx.Tx, id string, amount int64) error {
	tag, err := tx.Exec(ctx, `
		UPDATE wallets
		SET balance = balance + $1, updated_at = now()
		WHERE id = $2 AND status = 'ACTIVE'`,
		amount, id,
	)
	if err != nil {
		return fmt.Errorf("credit wallet: %w", err)
	}
	return mustAffectOne(tag, "credit wallet")
}
