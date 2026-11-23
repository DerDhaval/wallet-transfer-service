package handler

import (
	"log/slog"
	"net/http"

	"wallet-transfer-service/internal/middleware"
)

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	if h.ready != nil {
		if err := h.ready(r.Context()); err != nil {
			slog.Error("health check failed", "error", err, "request_id", middleware.RequestIDFrom(r.Context()))
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
