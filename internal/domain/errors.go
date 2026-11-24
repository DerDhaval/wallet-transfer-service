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
	ErrInvalidRequest = &Error{Status: 400, Code: "INVALID_REQUEST", Message: "Please check your request and try again"}
	ErrInvalidAmount  = &Error{Status: 400, Code: "INVALID_AMOUNT", Message: "Enter a valid amount as a whole number in the smallest currency unit (for example paise). It must be at least 1"}
	ErrSameWallet     = &Error{Status: 400, Code: "SAME_WALLET", Message: "You cannot transfer money to the same wallet. Choose a different destination wallet"}

	ErrWalletNotFound   = &Error{Status: 404, Code: "WALLET_NOT_FOUND", Message: "We could not find that wallet. Check the wallet id and try again"}
	ErrTransferNotFound = &Error{Status: 404, Code: "TRANSFER_NOT_FOUND", Message: "We could not find that transfer. Check the transfer id and try again"}

	ErrIdempotencyConflict = &Error{Status: 409, Code: "IDEMPOTENCY_CONFLICT", Message: "This idempotency key was already used for a different payment. Use a new key to send a new transfer"}
	ErrWalletExists        = &Error{Status: 409, Code: "WALLET_EXISTS", Message: "A wallet with this id already exists. Use a different wallet id"}

	ErrInsufficientFunds = &Error{Status: 422, Code: "INSUFFICIENT_FUNDS", Message: "Not enough balance in the sending wallet to complete this payment"}
	ErrWalletNotActive   = &Error{Status: 422, Code: "WALLET_NOT_ACTIVE", Message: "This wallet is not active, so money cannot be sent or received"}
	ErrAmountOverflow    = &Error{Status: 422, Code: "AMOUNT_OVERFLOW", Message: "This payment would push the receiving wallet balance past the allowed maximum"}

	ErrCurrencyNotSupported = &Error{Status: 422, Code: "CURRENCY_NOT_SUPPORTED", Message: "This currency is not supported. Use GET /currencies to see supported currencies"}
	ErrAmountTooSmall       = &Error{Status: 422, Code: "AMOUNT_TOO_SMALL", Message: "This amount is too small to convert into the receiving wallet's currency"}

	ErrUnsupportedMedia = &Error{Status: 415, Code: "UNSUPPORTED_MEDIA_TYPE", Message: "Send the request as JSON with Content-Type: application/json"}

	ErrTemporarilyUnavailable = &Error{Status: 503, Code: "TEMPORARILY_UNAVAILABLE", Message: "The payment could not be completed right now. Please retry in a moment"}
	ErrInternal               = &Error{Status: 500, Code: "INTERNAL_ERROR", Message: "Something went wrong on our side. Please try again"}
)

// AsError reports whether err is or wraps a domain error.
func AsError(err error) (*Error, bool) {
	var target *Error
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}

func WalletNotFound(id string) *Error {
	return ErrWalletNotFound.WithMessage(`Wallet "` + id + `" was not found. Check the wallet id and try again`)
}

func TransferNotFound(id string) *Error {
	return ErrTransferNotFound.WithMessage(`Transfer "` + id + `" was not found. Check the transfer id and try again`)
}
