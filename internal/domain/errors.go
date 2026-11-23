package domain

import "errors"

// Error is a client-visible failure. Status is the HTTP status the API returns.
// Domain carries the status because a committed idempotent response must store it.
type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// Is matches on the stable error code so a message with extra detail still compares equal.
func (e *Error) Is(target error) bool {
	other, ok := target.(*Error)
	if !ok || e == nil || other == nil {
		return false
	}
	return e.Code == other.Code
}

// WithMessage returns a copy with a more specific client message.
func (e *Error) WithMessage(message string) *Error {
	if e == nil {
		return nil
	}
	clone := *e
	clone.Message = message
	return &clone
}

var (
	ErrInvalidRequest = &Error{Status: 400, Code: "INVALID_REQUEST", Message: "request is invalid"}
	ErrInvalidAmount  = &Error{Status: 400, Code: "INVALID_AMOUNT", Message: "amount must be a positive integer in minor units"}
	ErrSameWallet     = &Error{Status: 400, Code: "SAME_WALLET", Message: "source and destination wallets must be different"}

	ErrWalletNotFound   = &Error{Status: 404, Code: "WALLET_NOT_FOUND", Message: "wallet was not found"}
	ErrTransferNotFound = &Error{Status: 404, Code: "TRANSFER_NOT_FOUND", Message: "transfer was not found"}

	ErrIdempotencyConflict = &Error{Status: 409, Code: "IDEMPOTENCY_CONFLICT", Message: "idempotency key was already used with a different request"}
	ErrWalletExists        = &Error{Status: 409, Code: "WALLET_EXISTS", Message: "wallet already exists"}

	ErrInsufficientFunds = &Error{Status: 422, Code: "INSUFFICIENT_FUNDS", Message: "source wallet has insufficient funds"}
	ErrWalletNotActive   = &Error{Status: 422, Code: "WALLET_NOT_ACTIVE", Message: "wallet is not active"}
	ErrCurrencyMismatch  = &Error{Status: 422, Code: "CURRENCY_MISMATCH", Message: "wallets must use the same currency"}
	ErrAmountOverflow    = &Error{Status: 422, Code: "AMOUNT_OVERFLOW", Message: "destination wallet balance would overflow"}

	ErrUnsupportedMedia = &Error{Status: 415, Code: "UNSUPPORTED_MEDIA_TYPE", Message: "Content-Type must be application/json"}

	ErrTemporarilyUnavailable = &Error{Status: 503, Code: "TEMPORARILY_UNAVAILABLE", Message: "the request could not be completed, please retry"}
	ErrInternal               = &Error{Status: 500, Code: "INTERNAL_ERROR", Message: "an unexpected error occurred"}
)

// AsError reports whether err is or wraps a domain error.
func AsError(err error) (*Error, bool) {
	var target *Error
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}
