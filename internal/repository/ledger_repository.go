package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"wallet-transfer-service/internal/domain"
)

// InsertLedgerPair posts the debit (Amount) and the credit (ToAmount) for one transfer.
func (s *Store) InsertLedgerPair(ctx context.Context, tx pgx.Tx, t domain.Transfer) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO ledger_entries (id, wallet_id, transfer_id, entry_type, amount)
		VALUES
			($1, $2, $3, 'DEBIT', $4),
			($5, $6, $3, 'CREDIT', $7)`,
		domain.NewID("led"), t.FromWalletID, t.ID, t.Amount,
		domain.NewID("led"), t.ToWalletID, t.ToAmount,
	)
	if err != nil {
		return fmt.Errorf("insert ledger entries: %w", err)
	}
	return nil
}

// ListLedgerEntries returns a wallet's postings, newest first. The currency is the wallet's own.
func (s *Store) ListLedgerEntries(ctx context.Context, walletID string, limit, offset int) ([]domain.LedgerEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT l.id, l.wallet_id, l.transfer_id, l.entry_type, l.amount, w.currency, l.created_at
		FROM ledger_entries l
		JOIN wallets w ON w.id = l.wallet_id
		WHERE l.wallet_id = $1
		ORDER BY l.created_at DESC, l.id DESC
		LIMIT $2 OFFSET $3`, walletID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list ledger entries: %w", err)
	}
	defer rows.Close()

	entries := make([]domain.LedgerEntry, 0)
	for rows.Next() {
		var entry domain.LedgerEntry
		var entryType string
		if err := rows.Scan(&entry.ID, &entry.WalletID, &entry.TransferID, &entryType, &entry.Amount, &entry.Currency, &entry.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan ledger entry: %w", err)
		}
		entry.EntryType = domain.EntryType(entryType)
		entry.CreatedAt = entry.CreatedAt.UTC()
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list ledger entries: %w", err)
	}
	return entries, nil
}
