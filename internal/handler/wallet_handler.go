package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"wallet-transfer-service/internal/domain"
	"wallet-transfer-service/internal/service"
)

type createWalletRequest struct {
	ID             string `json:"id"`
	Currency       string `json:"currency"`
	InitialBalance int64  `json:"initialBalance"`
}

func (h *Handler) CreateWallet(w http.ResponseWriter, r *http.Request) {
	var req createWalletRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	wallet, err := h.svc.CreateWallet(r.Context(), service.CreateWalletCommand{
		ID:             strings.TrimSpace(req.ID),
		Currency:       strings.TrimSpace(req.Currency),
		InitialBalance: req.InitialBalance,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, walletResponseFrom(wallet))
}

func (h *Handler) GetBalance(w http.ResponseWriter, r *http.Request) {
	wallet, err := h.svc.GetWallet(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, walletResponseFrom(wallet))
}

func (h *Handler) ListTransactions(w http.ResponseWriter, r *http.Request) {
	limit, offset, err := pageParams(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	walletID := r.PathValue("id")
	entries, err := h.svc.ListWalletEntries(r.Context(), walletID, limit, offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if limit == 0 {
		limit = 20
	}
	views := make([]ledgerEntryResponse, 0, len(entries))
	for _, entry := range entries {
		views = append(views, ledgerEntryResponseFrom(entry))
	}
	writeJSON(w, http.StatusOK, transactionsResponse{
		WalletID:     walletID,
		Transactions: views,
		Limit:        limit,
		Offset:       offset,
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	mediaType := r.Header.Get("Content-Type")
	if mediaType != "" && !strings.Contains(strings.ToLower(mediaType), "application/json") {
		return domain.ErrUnsupportedMedia
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxBytes *http.MaxBytesError
		if errors.As(err, &maxBytes) {
			return domain.ErrInvalidRequest.WithMessage("request body is too large")
		}
		if errors.Is(err, io.EOF) {
			return domain.ErrInvalidRequest.WithMessage("request body is required")
		}
		return domain.ErrInvalidRequest.WithMessage("invalid JSON body")
	}
	if dec.More() {
		return domain.ErrInvalidRequest.WithMessage("request body must contain a single JSON value")
	}
	return nil
}

func pageParams(r *http.Request) (int, int, error) {
	limit, err := optionalInt(r.URL.Query().Get("limit"))
	if err != nil {
		return 0, 0, domain.ErrInvalidRequest.WithMessage("limit must be an integer")
	}
	offset, err := optionalInt(r.URL.Query().Get("offset"))
	if err != nil {
		return 0, 0, domain.ErrInvalidRequest.WithMessage("offset must be an integer")
	}
	return limit, offset, nil
}

func optionalInt(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	return value, nil
}
