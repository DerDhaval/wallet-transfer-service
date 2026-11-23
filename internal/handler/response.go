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
	if !ok {
		slog.Error("request failed", "error", err, "path", r.URL.Path, "request_id", middleware.RequestIDFrom(r.Context()))
		apiErr = domain.ErrInternal
	} else if apiErr.Status >= 500 {
		slog.Error("request failed", "code", apiErr.Code, "message", apiErr.Message, "path", r.URL.Path, "request_id", middleware.RequestIDFrom(r.Context()))
	} else {
		slog.Warn("request rejected", "code", apiErr.Code, "message", apiErr.Message, "path", r.URL.Path, "status", apiErr.Status, "request_id", middleware.RequestIDFrom(r.Context()))
	}
	writeJSON(w, apiErr.Status, domain.ErrorBody(apiErr))
}
