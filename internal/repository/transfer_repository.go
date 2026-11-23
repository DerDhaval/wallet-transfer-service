package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"wallet-transfer-service/internal/domain"
)

const transferColumns = `id, from_wallet_id, to_wallet_id, amount, currency, status, failure_reason, created_at, updated_at`

func scanTransfer(row pgx.Row) (domain.Transfer, error) {
	var t domain.Transfer
	var status string
	if err := row.Scan(
		&t.ID, &t.FromWalletID, &t.ToWalletID, &t.Amount, &t.Currency,
		&status, &t.FailureReason, &t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		return domain.Transfer{}, err
	}
	t.Status = domain.TransferStatus(status)
	t.CreatedAt = utc(t.CreatedAt)
	t.UpdatedAt = utc(t.UpdatedAt)
	return t, nil
}

func (s *Store) InsertPendingTransfer(ctx context.Context, tx pgx.Tx, t *domain.Transfer) error {
	row := tx.QueryRow(ctx, `
		INSERT INTO transfers (id, from_wallet_id, to_wallet_id, amount, currency, status)
		VALUES ($1, $2, $3, $4, $5, 'PENDING')
		RETURNING `+transferColumns,
		t.ID, t.FromWalletID, t.ToWalletID, t.Amount, t.Currency,
	)
	scanned, err := scanTransfer(row)
	if err != nil {
		return fmt.Errorf("insert transfer: %w", err)
	}
	*t = scanned
	return nil
}

func (s *Store) MarkTransferProcessed(ctx context.Context, tx pgx.Tx, t *domain.Transfer) error {
	row := tx.QueryRow(ctx, `
		UPDATE transfers
		SET status = 'PROCESSED', failure_reason = NULL, updated_at = now()
		WHERE id = $1 AND status = 'PENDING'
		RETURNING `+transferColumns, t.ID)
	scanned, err := scanTransfer(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("mark transfer %s processed: status was not pending", t.ID)
	}
	if err != nil {
		return fmt.Errorf("mark transfer processed: %w", err)
	}
	*t = scanned
	return nil
}

func (s *Store) MarkTransferFailed(ctx context.Context, tx pgx.Tx, t *domain.Transfer, reason string) error {
	row := tx.QueryRow(ctx, `
		UPDATE transfers
		SET status = 'FAILED', failure_reason = $2, updated_at = now()
		WHERE id = $1 AND status = 'PENDING'
		RETURNING `+transferColumns, t.ID, reason)
	scanned, err := scanTransfer(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("mark transfer %s failed: status was not pending", t.ID)
	}
	if err != nil {
		return fmt.Errorf("mark transfer failed: %w", err)
	}
	*t = scanned
	return nil
}

func (s *Store) GetTransfer(ctx context.Context, id string) (domain.Transfer, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+transferColumns+` FROM transfers WHERE id = $1`, id)
	t, err := scanTransfer(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Transfer{}, domain.ErrTransferNotFound
	}
	if err != nil {
		return domain.Transfer{}, fmt.Errorf("get transfer: %w", err)
	}
	return t, nil
}
