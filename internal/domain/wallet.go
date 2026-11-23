package domain

import (
	"regexp"
	"time"
)

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

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

// ValidateNewWallet checks a wallet before it is inserted.
// emptyID is true when the caller asked the service to generate an id.
func ValidateNewWallet(id string, currency string, openingBalance int64, emptyID bool) error {
	if !emptyID {
		if err := ValidateWalletID(id); err != nil {
			return err
		}
	}
	if !currencyPattern.MatchString(currency) {
		return ErrInvalidRequest.WithMessage("Currency must be a 3-letter uppercase code such as INR or USD")
	}
	if openingBalance < 0 {
		return ErrInvalidAmount.WithMessage("Opening balance cannot be negative")
	}
	if openingBalance > MaxOpeningBalance {
		return ErrInvalidAmount.WithMessage("Opening balance is too large for this wallet")
	}
	return nil
}
