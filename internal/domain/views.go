package domain

import (
	"encoding/json"
	"time"
)

// JSON shapes shared by HTTP handlers and idempotent stored responses.

type TransferView struct {
	TransferID    string    `json:"transferId"`
	FromWalletID  string    `json:"fromWalletId"`
	ToWalletID    string    `json:"toWalletId"`
	Amount        int64     `json:"amount"`
	Currency      string    `json:"currency"`
	ToAmount      int64     `json:"toAmount,omitempty"`
	ToCurrency    string    `json:"toCurrency"`
	ExchangeRate  string    `json:"exchangeRate,omitempty"`
	Status        string    `json:"status"`
	FailureReason *string   `json:"failureReason,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type WalletView struct {
	WalletID  string    `json:"walletId"`
	Balance   int64     `json:"balance"`
	Currency  string    `json:"currency"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type LedgerView struct {
	EntryID    string    `json:"entryId"`
	WalletID   string    `json:"walletId"`
	TransferID string    `json:"transferId"`
	EntryType  string    `json:"entryType"`
	Amount     int64     `json:"amount"`
	Currency   string    `json:"currency"`
	CreatedAt  time.Time `json:"createdAt"`
}

func ViewTransfer(t Transfer) TransferView {
	return TransferView{
		TransferID:    t.ID,
		FromWalletID:  t.FromWalletID,
		ToWalletID:    t.ToWalletID,
		Amount:        t.Amount,
		Currency:      t.Currency,
		ToAmount:      t.ToAmount,
		ToCurrency:    t.ToCurrency,
		ExchangeRate:  t.ExchangeRate,
		Status:        string(t.Status),
		FailureReason: t.FailureReason,
		CreatedAt:     t.CreatedAt.UTC(),
		UpdatedAt:     t.UpdatedAt.UTC(),
	}
}

func ViewWallet(w Wallet) WalletView {
	return WalletView{
		WalletID:  w.ID,
		Balance:   w.Balance,
		Currency:  w.Currency,
		Status:    string(w.Status),
		CreatedAt: w.CreatedAt.UTC(),
		UpdatedAt: w.UpdatedAt.UTC(),
	}
}

func ViewLedger(e LedgerEntry) LedgerView {
	return LedgerView{
		EntryID:    e.ID,
		WalletID:   e.WalletID,
		TransferID: e.TransferID,
		EntryType:  string(e.EntryType),
		Amount:     e.Amount,
		Currency:   e.Currency,
		CreatedAt:  e.CreatedAt.UTC(),
	}
}

func ErrorBody(err *Error) map[string]any {
	return map[string]any{
		"error": map[string]string{
			"code":    err.Code,
			"message": err.Message,
		},
	}
}

func MarshalJSONLine(v any) ([]byte, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

func MarshalTransferOK(t Transfer) ([]byte, error) {
	return MarshalJSONLine(ViewTransfer(t))
}

func MarshalTransferFailed(t Transfer, cause *Error) ([]byte, error) {
	body := ErrorBody(cause)
	body["transfer"] = ViewTransfer(t)
	return MarshalJSONLine(body)
}
