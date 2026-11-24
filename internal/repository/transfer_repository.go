package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"wallet-transfer-service/internal/domain"
)

// to_amount and exchange_rate are NULL for a failed attempt that never reached a valid conversion.
// The domain uses 0 and "" for that case.
const transferColumns = `id, from_wallet_id, to_wallet_id, amount, currency, COALESCE(to_amount, 0), to_currency, COALESCE(exchange_rate, ''), status, failure_reason, created_at, updated_at`

func scanTransfer(row pgx.Row) (domain.Transfer, error) {
	var t domain.Transfer
	var status string
	if err := row.Scan(
		&t.ID, &t.FromWalletID, &t.ToWalletID, &t.Amount, &t.Currency,
		&t.ToAmount, &t.ToCurrency, &t.ExchangeRate,
		&status, &t.FailureReason, &t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		return domain.Transfer{}, err
	}
	t.Status = domain.TransferStatus(status)
	t.CreatedAt = t.CreatedAt.UTC()
	t.UpdatedAt = t.UpdatedAt.UTC()
	return t, nil
}

func (s *Store) InsertPendingTransfer(ctx context.Context, tx pgx.Tx, t *domain.Transfer) error {
	row := tx.QueryRow(ctx, `
		INSERT INTO transfers (id, from_wallet_id, to_wallet_id, amount, currency, to_amount, to_currency, exchange_rate, status)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6::bigint, 0), $7, NULLIF($8::text, ''), 'PENDING')
		RETURNING `+transferColumns,
		t.ID, t.FromWalletID, t.ToWalletID, t.Amount, t.Currency, t.ToAmount, t.ToCurrency, t.ExchangeRate,
	)
	scanned, err := scanTransfer(row)
	if err != nil {
		return fmt.Errorf("insert transfer: %w", err)
	}
	*t = scanned
	return nil
}

func (s *Store) MarkTransferProcessed(ctx context.Context, tx pgx.Tx, t *domain.Transfer) error {
	return s.finalizeTransfer(ctx, tx, t, `
		UPDATE transfers
		SET status = 'PROCESSED', failure_reason = NULL, updated_at = now()
		WHERE id = $1 AND status = 'PENDING'
		RETURNING `+transferColumns, t.ID)
}

func (s *Store) MarkTransferFailed(ctx context.Context, tx pgx.Tx, t *domain.Transfer, reason string) error {
	return s.finalizeTransfer(ctx, tx, t, `
		UPDATE transfers
		SET status = 'FAILED', failure_reason = $2, updated_at = now()
		WHERE id = $1 AND status = 'PENDING'
		RETURNING `+transferColumns, t.ID, reason)
}

func (s *Store) finalizeTransfer(ctx context.Context, tx pgx.Tx, t *domain.Transfer, query string, args ...any) error {
	scanned, err := scanTransfer(tx.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("finalize transfer %s: status was not pending", t.ID)
	}
	if err != nil {
		return fmt.Errorf("finalize transfer: %w", err)
	}
	*t = scanned
	return nil
}

func (s *Store) GetTransfer(ctx context.Context, id string) (domain.Transfer, error) {
	t, err := scanTransfer(s.pool.QueryRow(ctx, `SELECT `+transferColumns+` FROM transfers WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Transfer{}, domain.TransferNotFound(id)
	}
	if err != nil {
		return domain.Transfer{}, fmt.Errorf("get transfer: %w", err)
	}
	return t, nil
}
