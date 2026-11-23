package domain

import "time"

type EntryType string

const (
	EntryDebit  EntryType = "DEBIT"
	EntryCredit EntryType = "CREDIT"
)

type LedgerEntry struct {
	ID         string
	WalletID   string
	TransferID string
	EntryType  EntryType
	Amount     int64
	CreatedAt  time.Time
}
