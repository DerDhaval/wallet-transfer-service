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
	writeJSON(w, http.StatusCreated, domain.ViewWallet(wallet))
}

func (h *Handler) GetBalance(w http.ResponseWriter, r *http.Request) {
	wallet, err := h.svc.GetWallet(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, domain.ViewWallet(wallet))
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
		limit = domain.DefaultPageLimit
	}
	views := make([]domain.LedgerView, 0, len(entries))
	for _, entry := range entries {
		views = append(views, domain.ViewLedger(entry))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"walletId":     walletID,
		"transactions": views,
		"limit":        limit,
		"offset":       offset,
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
			return domain.ErrInvalidRequest.WithMessage("Request body is too large. Keep it under 1 MB")
		}
		if errors.Is(err, io.EOF) {
			return domain.ErrInvalidRequest.WithMessage("Request body is required. Send a JSON object")
		}
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) {
			field := typeErr.Field
			if field == "" {
				field = "a field"
			}
			return domain.ErrInvalidRequest.WithMessage(field + " has the wrong type. Check the value and try again")
		}
		msg := err.Error()
		if strings.Contains(msg, "unknown field") {
			field := strings.Trim(strings.TrimPrefix(msg, "json: unknown field "), `"`)
			if field == "" {
				field = "extra field"
			}
			return domain.ErrInvalidRequest.WithMessage(`Unknown field "` + field + `". Remove it and try again`)
		}
		return domain.ErrInvalidRequest.WithMessage("Request body is not valid JSON. Check the format and try again")
	}
	if dec.More() {
		return domain.ErrInvalidRequest.WithMessage("Send only one JSON object in the request body")
	}
	return nil
}

func pageParams(r *http.Request) (int, int, error) {
	limit, err := optionalInt(r.URL.Query().Get("limit"))
	if err != nil {
		return 0, 0, domain.ErrInvalidRequest.WithMessage("limit must be a whole number")
	}
	offset, err := optionalInt(r.URL.Query().Get("offset"))
	if err != nil {
		return 0, 0, domain.ErrInvalidRequest.WithMessage("offset must be a whole number")
	}
	return limit, offset, nil
}

func optionalInt(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	return strconv.Atoi(raw)
}
