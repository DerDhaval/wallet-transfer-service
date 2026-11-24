package domain

import "time"

type WalletStatus string

const (
	WalletActive WalletStatus = "ACTIVE"
	WalletFrozen WalletStatus = "FROZEN"
	WalletClosed WalletStatus = "CLOSED"
)

type Wallet struct {
	ID             string
	Balance        int64
	OpeningBalance int64
	Currency       string
	Status         WalletStatus
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ValidateNewWallet checks a wallet before it is inserted.
// emptyID is true when the caller asked the service to generate an id.
func ValidateNewWallet(id string, currency string, openingBalance int64, emptyID bool) error {
	if !emptyID {
		if err := ValidateWalletID(id); err != nil {
			return err
		}
	}
	if _, ok := LookupCurrency(currency); !ok {
		return ErrInvalidRequest.WithMessage(
			"Currency " + currency + " is not supported. Use GET /currencies to see supported currencies")
	}
	if openingBalance < 0 {
		return ErrInvalidAmount.WithMessage("Opening balance cannot be negative")
	}
	if openingBalance > MaxOpeningBalance {
		return ErrInvalidAmount.WithMessage("Opening balance is too large for this wallet")
	}
	return nil
}
