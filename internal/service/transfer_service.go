package service

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
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
				logger(ctx).Warn("idempotency conflict", "idempotency_key", cmd.IdempotencyKey)
				return domain.ErrIdempotencyConflict
			}
			if existing.ResponseStatus == 0 || len(existing.ResponseBody) == 0 {
				return fmt.Errorf("idempotency key %s is incomplete", cmd.IdempotencyKey)
			}
			result = Result{
				StatusCode: existing.ResponseStatus,
				Body:       existing.ResponseBody,
				Replayed:   true,
			}
			transferID := ""
			if existing.TransferID != nil {
				transferID = *existing.TransferID
			}
			logger(ctx).Info("idempotent replay", "idempotency_key", cmd.IdempotencyKey, "transfer_id", transferID)
			return nil
		}

		outcome, err := s.executeClaimed(ctx, tx, cmd)
		if err != nil {
			return err
		}
		result = outcome
		if s.beforeCommit != nil {
			if err := s.beforeCommit(ctx); err != nil {
				logger(ctx).Error("transfer transaction aborted before commit", "idempotency_key", cmd.IdempotencyKey, "error", err)
				return err
			}
		}
		return nil
	})
	if err != nil {
		mapped := mapTxErr(err)
		if _, ok := domain.AsError(mapped); !ok {
			logger(ctx).Error("transfer transaction failed", "idempotency_key", cmd.IdempotencyKey, "error", err)
		}
		return Result{}, mapped
	}
	return result, nil
}

func (s *Service) executeClaimed(ctx context.Context, tx pgx.Tx, cmd CreateTransferCommand) (Result, error) {
	first, second := domain.OrderedWalletIDs(cmd.FromWalletID, cmd.ToWalletID)
	left, err := s.store.GetWalletForUpdate(ctx, tx, first)
	if err != nil {
		return Result{}, err
	}
	right, err := s.store.GetWalletForUpdate(ctx, tx, second)
	if err != nil {
		return Result{}, err
	}
	wallets := map[string]domain.Wallet{left.ID: left, right.ID: right}
	from, ok := wallets[cmd.FromWalletID]
	if !ok {
		return Result{}, fmt.Errorf("source wallet %s was not locked", cmd.FromWalletID)
	}
	to, ok := wallets[cmd.ToWalletID]
	if !ok {
		return Result{}, fmt.Errorf("destination wallet %s was not locked", cmd.ToWalletID)
	}

	transfer := domain.Transfer{
		ID:           domain.NewID("trf"),
		FromWalletID: from.ID,
		ToWalletID:   to.ID,
		Amount:       cmd.Amount,
		Currency:     from.Currency,
		Status:       domain.StatusPending,
	}

	switch {
	case from.Status != domain.WalletActive:
		return s.recordFailure(ctx, tx, cmd.IdempotencyKey, transfer, domain.ErrWalletNotActive.WithMessage("wallet "+from.ID+" is not active"))
	case to.Status != domain.WalletActive:
		return s.recordFailure(ctx, tx, cmd.IdempotencyKey, transfer, domain.ErrWalletNotActive.WithMessage("wallet "+to.ID+" is not active"))
	case from.Currency != to.Currency:
		return s.recordFailure(ctx, tx, cmd.IdempotencyKey, transfer, domain.ErrCurrencyMismatch)
	case from.Balance < cmd.Amount:
		return s.recordFailure(ctx, tx, cmd.IdempotencyKey, transfer, domain.ErrInsufficientFunds)
	case to.Balance > math.MaxInt64-cmd.Amount:
		return s.recordFailure(ctx, tx, cmd.IdempotencyKey, transfer, domain.ErrAmountOverflow)
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
	body, err := processedBody(transfer)
	if err != nil {
		return Result{}, err
	}
	if err := s.store.CompleteIdempotency(ctx, tx, cmd.IdempotencyKey, transfer.ID, 201, string(body)); err != nil {
		return Result{}, err
	}
	logger(ctx).Info("transfer processed",
		"transfer_id", transfer.ID,
		"from_wallet_id", from.ID,
		"to_wallet_id", to.ID,
		"amount", cmd.Amount,
		"currency", transfer.Currency,
	)
	return Result{StatusCode: 201, Body: body}, nil
}

func (s *Service) recordFailure(ctx context.Context, tx pgx.Tx, key string, transfer domain.Transfer, cause *domain.Error) (Result, error) {
	if err := s.store.InsertPendingTransfer(ctx, tx, &transfer); err != nil {
		return Result{}, err
	}
	if err := s.store.MarkTransferFailed(ctx, tx, &transfer, cause.Code); err != nil {
		return Result{}, err
	}
	body, err := failedBody(transfer, cause)
	if err != nil {
		return Result{}, err
	}
	if err := s.store.CompleteIdempotency(ctx, tx, key, transfer.ID, cause.Status, string(body)); err != nil {
		return Result{}, err
	}
	logger(ctx).Warn("transfer failed",
		"transfer_id", transfer.ID,
		"from_wallet_id", transfer.FromWalletID,
		"to_wallet_id", transfer.ToWalletID,
		"amount", transfer.Amount,
		"code", cause.Code,
	)
	return Result{StatusCode: cause.Status, Body: body}, nil
}

func (s *Service) GetTransfer(ctx context.Context, id string) (domain.Transfer, error) {
	if err := domain.ValidateWalletID(id); err != nil {
		return domain.Transfer{}, domain.ErrInvalidRequest.WithMessage("transfer id must be 1-64 characters of letters, digits, underscore, or hyphen")
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
