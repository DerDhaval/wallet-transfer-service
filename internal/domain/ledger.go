package domain

import "time"

type EntryType string

const (
	EntryDebit  EntryType = "DEBIT"
	EntryCredit EntryType = "CREDIT"
)

// LedgerEntry is one posting. Amount is in minor units of Currency, the wallet's currency.
type LedgerEntry struct {
	ID         string
	WalletID   string
	TransferID string
	EntryType  EntryType
	Amount     int64
	Currency   string
	CreatedAt  time.Time
}
