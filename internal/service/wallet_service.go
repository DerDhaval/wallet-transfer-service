package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5/pgconn"

	"wallet-transfer-service/internal/domain"
)

type CreateWalletCommand struct {
	ID             string
	Currency       string
	InitialBalance int64
}

func (s *Service) CreateWallet(ctx context.Context, cmd CreateWalletCommand) (domain.Wallet, error) {
	generated := cmd.ID == ""
	if err := domain.ValidateNewWallet(cmd.ID, cmd.Currency, cmd.InitialBalance, generated); err != nil {
		return domain.Wallet{}, err
	}
	id := cmd.ID
	if generated {
		id = domain.NewID("wal")
	}
	wallet := domain.Wallet{
		ID:             id,
		Balance:        cmd.InitialBalance,
		OpeningBalance: cmd.InitialBalance,
		Currency:       cmd.Currency,
		Status:         domain.WalletActive,
	}
	if err := s.store.InsertWallet(ctx, &wallet); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.Wallet{}, domain.ErrWalletExists
		}
		return domain.Wallet{}, err
	}
	slog.Info("wallet created",
		"wallet_id", wallet.ID,
		"currency", wallet.Currency,
		"opening_balance", wallet.OpeningBalance,
	)
	return wallet, nil
}

func (s *Service) GetWallet(ctx context.Context, id string) (domain.Wallet, error) {
	if err := domain.ValidateWalletID(id); err != nil {
		return domain.Wallet{}, err
	}
	return s.store.GetWallet(ctx, id)
}

func (s *Service) ListWalletEntries(ctx context.Context, walletID string, limit, offset int) ([]domain.LedgerEntry, error) {
	if err := domain.ValidateWalletID(walletID); err != nil {
		return nil, err
	}
	limit, offset, err := normalizePage(limit, offset)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.GetWallet(ctx, walletID); err != nil {
		return nil, err
	}
	return s.store.ListLedgerEntries(ctx, walletID, limit, offset)
}

func normalizePage(limit, offset int) (int, int, error) {
	if limit == 0 {
		limit = domain.DefaultPageLimit
	}
	if limit < 1 || limit > domain.MaxPageLimit {
		return 0, 0, domain.ErrInvalidRequest.WithMessage("limit must be between 1 and 100")
	}
	if offset < 0 {
		return 0, 0, domain.ErrInvalidRequest.WithMessage("offset must be 0 or greater")
	}
	return limit, offset, nil
}
