package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"wallet-transfer-service/internal/domain"
)

func (s *Store) InsertLedgerPair(ctx context.Context, tx pgx.Tx, t domain.Transfer) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO ledger_entries (id, wallet_id, transfer_id, entry_type, amount)
		VALUES
			($1, $2, $3, 'DEBIT', $4),
			($5, $6, $3, 'CREDIT', $4)`,
		domain.NewID("led"), t.FromWalletID, t.ID, t.Amount,
		domain.NewID("led"), t.ToWalletID,
	)
	if err != nil {
		return fmt.Errorf("insert ledger entries: %w", err)
	}
	return nil
}

func (s *Store) ListLedgerEntries(ctx context.Context, walletID string, limit, offset int) ([]domain.LedgerEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, wallet_id, transfer_id, entry_type, amount, created_at
		FROM ledger_entries
		WHERE wallet_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3`, walletID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list ledger entries: %w", err)
	}
	defer rows.Close()

	var entries []domain.LedgerEntry
	for rows.Next() {
		var entry domain.LedgerEntry
		var entryType string
		if err := rows.Scan(&entry.ID, &entry.WalletID, &entry.TransferID, &entryType, &entry.Amount, &entry.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan ledger entry: %w", err)
		}
		entry.EntryType = domain.EntryType(entryType)
		entry.CreatedAt = utc(entry.CreatedAt)
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list ledger entries: %w", err)
	}
	if entries == nil {
		entries = []domain.LedgerEntry{}
	}
	return entries, nil
}
