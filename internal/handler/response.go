package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"wallet-transfer-service/internal/domain"
	"wallet-transfer-service/internal/middleware"
)

type transferResponse struct {
	TransferID    string    `json:"transferId"`
	FromWalletID  string    `json:"fromWalletId"`
	ToWalletID    string    `json:"toWalletId"`
	Amount        int64     `json:"amount"`
	Currency      string    `json:"currency"`
	Status        string    `json:"status"`
	FailureReason *string   `json:"failureReason,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type walletResponse struct {
	WalletID  string    `json:"walletId"`
	Balance   int64     `json:"balance"`
	Currency  string    `json:"currency"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ledgerEntryResponse struct {
	EntryID    string    `json:"entryId"`
	WalletID   string    `json:"walletId"`
	TransferID string    `json:"transferId"`
	EntryType  string    `json:"entryType"`
	Amount     int64     `json:"amount"`
	CreatedAt  time.Time `json:"createdAt"`
}

type transactionsResponse struct {
	WalletID     string                `json:"walletId"`
	Transactions []ledgerEntryResponse `json:"transactions"`
	Limit        int                   `json:"limit"`
	Offset       int                   `json:"offset"`
}

func transferResponseFrom(t domain.Transfer) transferResponse {
	return transferResponse{
		TransferID:    t.ID,
		FromWalletID:  t.FromWalletID,
		ToWalletID:    t.ToWalletID,
		Amount:        t.Amount,
		Currency:      t.Currency,
		Status:        string(t.Status),
		FailureReason: t.FailureReason,
		CreatedAt:     t.CreatedAt.UTC(),
		UpdatedAt:     t.UpdatedAt.UTC(),
	}
}

func walletResponseFrom(w domain.Wallet) walletResponse {
	return walletResponse{
		WalletID:  w.ID,
		Balance:   w.Balance,
		Currency:  w.Currency,
		Status:    string(w.Status),
		CreatedAt: w.CreatedAt.UTC(),
		UpdatedAt: w.UpdatedAt.UTC(),
	}
}

func ledgerEntryResponseFrom(e domain.LedgerEntry) ledgerEntryResponse {
	return ledgerEntryResponse{
		EntryID:    e.ID,
		WalletID:   e.WalletID,
		TransferID: e.TransferID,
		EntryType:  string(e.EntryType),
		Amount:     e.Amount,
		CreatedAt:  e.CreatedAt.UTC(),
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	payload, err := json.Marshal(body)
	if err != nil {
		slog.Error("encode response", "error", err)
		http.Error(w, `{"error":{"code":"INTERNAL_ERROR","message":"an unexpected error occurred"}}`+"\n", http.StatusInternalServerError)
		return
	}
	payload = append(payload, '\n')
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	apiErr, ok := domain.AsError(err)
	if !ok || apiErr.Status >= 500 {
		slog.Error("request failed", "error", err, "path", r.URL.Path, "request_id", middleware.RequestIDFrom(r.Context()))
		if !ok {
			apiErr = domain.ErrInternal
		}
	}
	writeJSON(w, apiErr.Status, map[string]any{
		"error": map[string]string{
			"code":    apiErr.Code,
			"message": apiErr.Message,
		},
	})
}
