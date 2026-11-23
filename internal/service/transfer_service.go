package service

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"math"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"wallet-transfer-service/internal/domain"
)

type CreateTransferCommand struct {
	IdempotencyKey string
	FromWalletID   string
	ToWalletID     string
	Amount         int64
}

// Result is the HTTP response that was committed, or the stored response on replay.
type Result struct {
	StatusCode int
	Body       []byte
	Replayed   bool
}

func (s *Service) CreateTransfer(ctx context.Context, cmd CreateTransferCommand) (Result, error) {
	if err := domain.ValidateCreateTransfer(cmd.IdempotencyKey, cmd.FromWalletID, cmd.ToWalletID, cmd.Amount); err != nil {
		return Result{}, err
	}
	hash := domain.RequestHash(cmd.FromWalletID, cmd.ToWalletID, cmd.Amount)

	var result Result
	err := s.store.WithTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		claimed, existing, err := s.store.ClaimIdempotency(ctx, tx, cmd.IdempotencyKey, hash)
		if err != nil {
			return err
		}
		if !claimed {
			if subtle.ConstantTimeCompare([]byte(existing.RequestHash), []byte(hash)) != 1 {
				return domain.ErrIdempotencyConflict
			}
			if existing.ResponseStatus == 0 || len(existing.ResponseBody) == 0 {
				return fmt.Errorf("idempotency key %s is incomplete", cmd.IdempotencyKey)
			}
			result = Result{StatusCode: existing.ResponseStatus, Body: existing.ResponseBody, Replayed: true}
			slog.Info("transfer replayed",
				"idempotency_key", cmd.IdempotencyKey,
				"status", existing.ResponseStatus,
			)
			return nil
		}

		outcome, err := s.executeClaimed(ctx, tx, cmd)
		if err != nil {
			return err
		}
		result = outcome
		if s.beforeCommit != nil {
			return s.beforeCommit(ctx)
		}
		return nil
	})
	if err != nil {
		return Result{}, mapTxErr(err)
	}
	return result, nil
}

func (s *Service) executeClaimed(ctx context.Context, tx pgx.Tx, cmd CreateTransferCommand) (Result, error) {
	from, to, err := s.lockWallets(ctx, tx, cmd.FromWalletID, cmd.ToWalletID)
	if err != nil {
		return Result{}, err
	}

	transfer := domain.Transfer{
		ID:           domain.NewID("trf"),
		FromWalletID: from.ID,
		ToWalletID:   to.ID,
		Amount:       cmd.Amount,
		Currency:     from.Currency,
		Status:       domain.StatusPending,
	}

	if cause := transferBusinessError(from, to, cmd.Amount); cause != nil {
		return s.finish(ctx, tx, cmd.IdempotencyKey, transfer, cause)
	}

	if err := s.store.InsertPendingTransfer(ctx, tx, &transfer); err != nil {
		return Result{}, err
	}
	if err := s.store.DebitWallet(ctx, tx, from.ID, cmd.Amount); err != nil {
		return Result{}, err
	}
	if err := s.store.CreditWallet(ctx, tx, to.ID, cmd.Amount); err != nil {
		return Result{}, err
	}
	if err := s.store.InsertLedgerPair(ctx, tx, transfer); err != nil {
		return Result{}, err
	}
	if err := s.store.MarkTransferProcessed(ctx, tx, &transfer); err != nil {
		return Result{}, err
	}
	body, err := domain.MarshalTransferOK(transfer)
	if err != nil {
		return Result{}, err
	}
	if err := s.store.CompleteIdempotency(ctx, tx, cmd.IdempotencyKey, transfer.ID, 201, string(body)); err != nil {
		return Result{}, err
	}
	slog.Info("transfer processed",
		"transfer_id", transfer.ID,
		"from_wallet_id", from.ID,
		"to_wallet_id", to.ID,
		"amount", cmd.Amount,
		"currency", transfer.Currency,
		"idempotency_key", cmd.IdempotencyKey,
	)
	return Result{StatusCode: 201, Body: body}, nil
}

func (s *Service) lockWallets(ctx context.Context, tx pgx.Tx, fromID, toID string) (domain.Wallet, domain.Wallet, error) {
	first, second := domain.OrderedWalletIDs(fromID, toID)
	left, err := s.store.GetWalletForUpdate(ctx, tx, first)
	if err != nil {
		return domain.Wallet{}, domain.Wallet{}, err
	}
	right, err := s.store.GetWalletForUpdate(ctx, tx, second)
	if err != nil {
		return domain.Wallet{}, domain.Wallet{}, err
	}
	if left.ID == fromID {
		return left, right, nil
	}
	return right, left, nil
}

func transferBusinessError(from, to domain.Wallet, amount int64) *domain.Error {
	switch {
	case from.Status != domain.WalletActive:
		return domain.ErrWalletNotActive.WithMessage("Sending wallet " + from.ID + " is not active, so this payment cannot be sent")
	case to.Status != domain.WalletActive:
		return domain.ErrWalletNotActive.WithMessage("Receiving wallet " + to.ID + " is not active, so this payment cannot be received")
	case from.Currency != to.Currency:
		return domain.ErrCurrencyMismatch.WithMessage("Cannot transfer between " + from.Currency + " and " + to.Currency + ". Both wallets must use the same currency")
	case from.Balance < amount:
		return domain.ErrInsufficientFunds.WithMessage("Not enough balance in the sending wallet to complete this payment")
	case to.Balance > math.MaxInt64-amount:
		return domain.ErrAmountOverflow.WithMessage("This payment would push the receiving wallet balance past the allowed maximum")
	}
	return nil
}

func (s *Service) finish(ctx context.Context, tx pgx.Tx, key string, transfer domain.Transfer, cause *domain.Error) (Result, error) {
	if err := s.store.InsertPendingTransfer(ctx, tx, &transfer); err != nil {
		return Result{}, err
	}
	if err := s.store.MarkTransferFailed(ctx, tx, &transfer, cause.Code); err != nil {
		return Result{}, err
	}
	body, err := domain.MarshalTransferFailed(transfer, cause)
	if err != nil {
		return Result{}, err
	}
	if err := s.store.CompleteIdempotency(ctx, tx, key, transfer.ID, cause.Status, string(body)); err != nil {
		return Result{}, err
	}
	slog.Warn("transfer failed",
		"transfer_id", transfer.ID,
		"from_wallet_id", transfer.FromWalletID,
		"to_wallet_id", transfer.ToWalletID,
		"amount", transfer.Amount,
		"code", cause.Code,
		"idempotency_key", key,
	)
	return Result{StatusCode: cause.Status, Body: body}, nil
}

func (s *Service) GetTransfer(ctx context.Context, id string) (domain.Transfer, error) {
	if err := domain.ValidateWalletID(id); err != nil {
		return domain.Transfer{}, domain.ErrInvalidRequest.WithMessage("Transfer id is invalid. Use 1–64 letters, digits, underscore, or hyphen")
	}
	return s.store.GetTransfer(ctx, id)
}

func mapTxErr(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := domain.AsError(err); ok {
		return err
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "55P03", "40001", "40P01", "57014":
			return domain.ErrTemporarilyUnavailable
		}
	}
	return fmt.Errorf("execute transfer: %w", err)
}
