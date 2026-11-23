// Package router wires HTTP routes to handlers and applies middleware.
package router

import (
	"net/http"

	"wallet-transfer-service/internal/handler"
	"wallet-transfer-service/internal/middleware"
)

// New registers the API routes and wraps them with middleware.
func New(h *handler.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.Health)
	mux.HandleFunc("POST /wallets", h.CreateWallet)
	mux.HandleFunc("GET /wallets/{id}/balance", h.GetBalance)
	mux.HandleFunc("GET /wallets/{id}/transactions", h.ListTransactions)
	mux.HandleFunc("POST /transfers", h.CreateTransfer)
	mux.HandleFunc("GET /transfers/{id}", h.GetTransfer)

	return middleware.RequestID(mux)
}
