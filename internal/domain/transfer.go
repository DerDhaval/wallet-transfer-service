package domain

import (
	"regexp"
	"time"
)

type TransferStatus string

const (
	StatusPending   TransferStatus = "PENDING"
	StatusProcessed TransferStatus = "PROCESSED"
	StatusFailed    TransferStatus = "FAILED"
)

type Transfer struct {
	ID            string
	FromWalletID  string
	ToWalletID    string
	Amount        int64
	Currency      string
	Status        TransferStatus
	FailureReason *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

var (
	walletIDPattern       = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,255}$`)
)

func ValidateWalletID(id string) error {
	if !walletIDPattern.MatchString(id) {
		return ErrInvalidRequest.WithMessage("Wallet id is invalid. Use 1–64 letters, digits, underscore, or hyphen")
	}
	return nil
}

// ValidateCreateTransfer checks the request before a database transaction starts.
// Failures here do not consume the idempotency key.
func ValidateCreateTransfer(idempotencyKey, fromWalletID, toWalletID string, amount int64) error {
	if !idempotencyKeyPattern.MatchString(idempotencyKey) {
		return ErrInvalidRequest.WithMessage("Idempotency key is invalid. Use 1–255 letters, digits, or . _ : -")
	}
	if err := ValidateWalletID(fromWalletID); err != nil {
		return ErrInvalidRequest.WithMessage("fromWalletId is invalid. Use 1–64 letters, digits, underscore, or hyphen")
	}
	if err := ValidateWalletID(toWalletID); err != nil {
		return ErrInvalidRequest.WithMessage("toWalletId is invalid. Use 1–64 letters, digits, underscore, or hyphen")
	}
	if fromWalletID == toWalletID {
		return ErrSameWallet
	}
	switch {
	case amount == 0:
		return ErrInvalidAmount.WithMessage("Transfer amount must be greater than zero")
	case amount < 0:
		return ErrInvalidAmount.WithMessage("Transfer amount cannot be negative")
	case amount > MaxTransferAmount:
		return ErrInvalidAmount.WithMessage("Transfer amount is too large. Maximum allowed is 1000000000000")
	}
	return nil
}

// OrderedWalletIDs returns wallet ids in the only order the service may lock them.
// Every lock path must use this. Request order would deadlock when A pays B and B pays A.
func OrderedWalletIDs(a, b string) (string, string) {
	if a < b {
		return a, b
	}
	return b, a
}
