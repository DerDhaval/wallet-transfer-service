package handler

import (
	"context"

	"wallet-transfer-service/internal/domain"
	"wallet-transfer-service/internal/service"
)

// Service is the application API the HTTP layer depends on.
type Service interface {
	CreateTransfer(ctx context.Context, cmd service.CreateTransferCommand) (service.Result, error)
	GetTransfer(ctx context.Context, id string) (domain.Transfer, error)
	CreateWallet(ctx context.Context, cmd service.CreateWalletCommand) (domain.Wallet, error)
	GetWallet(ctx context.Context, id string) (domain.Wallet, error)
	ListWalletEntries(ctx context.Context, walletID string, limit, offset int) ([]domain.LedgerEntry, error)
}

// Handler is the HTTP adapter. Routes live in internal/router.
type Handler struct {
	svc   Service
	ready func(context.Context) error
}

func New(svc Service, ready func(context.Context) error) *Handler {
	return &Handler{svc: svc, ready: ready}
}
