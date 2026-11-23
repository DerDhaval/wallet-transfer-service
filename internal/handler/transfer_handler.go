package handler

import (
	"net/http"
	"strings"

	"wallet-transfer-service/internal/service"
)

type createTransferRequest struct {
	IdempotencyKey string `json:"idempotencyKey"`
	FromWalletID   string `json:"fromWalletId"`
	ToWalletID     string `json:"toWalletId"`
	Amount         int64  `json:"amount"`
}

func (h *Handler) CreateTransfer(w http.ResponseWriter, r *http.Request) {
	var req createTransferRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	result, err := h.svc.CreateTransfer(r.Context(), service.CreateTransferCommand{
		IdempotencyKey: strings.TrimSpace(req.IdempotencyKey),
		FromWalletID:   strings.TrimSpace(req.FromWalletID),
		ToWalletID:     strings.TrimSpace(req.ToWalletID),
		Amount:         req.Amount,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	if result.Replayed {
		w.Header().Set("X-Idempotent-Replay", "true")
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(result.StatusCode)
	_, _ = w.Write(result.Body)
}

func (h *Handler) GetTransfer(w http.ResponseWriter, r *http.Request) {
	transfer, err := h.svc.GetTransfer(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, transferResponseFrom(transfer))
}
