package handler

import (
	"log/slog"
	"net/http"

	"wallet-transfer-service/internal/domain"
	"wallet-transfer-service/internal/middleware"
)

func writeJSON(w http.ResponseWriter, status int, body any) {
	payload, err := domain.MarshalJSONLine(body)
	if err != nil {
		slog.Error("encode response", "error", err)
		http.Error(w, `{"error":{"code":"INTERNAL_ERROR","message":"Something went wrong on our side. Please try again"}}`+"\n", http.StatusInternalServerError)
		return
	}
	writeBytes(w, status, payload)
}

func writeBytes(w http.ResponseWriter, status int, payload []byte) {
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
	writeJSON(w, apiErr.Status, domain.ErrorBody(apiErr))
}
