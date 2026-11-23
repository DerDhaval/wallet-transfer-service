package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"wallet-transfer-service/internal/domain"
)

type IdempotencyRecord struct {
	ID             string
	Key            string
	RequestHash    string
	TransferID     *string
	ResponseStatus int
	ResponseBody   []byte
}

// ClaimIdempotency inserts the key. A false claim means another committed request already owns it.
// A concurrent owner blocks this insert until it commits or rolls back; that is the duplicate-request lock.
func (s *Store) ClaimIdempotency(ctx context.Context, tx pgx.Tx, key, hash string) (bool, IdempotencyRecord, error) {
	record := IdempotencyRecord{}
	err := tx.QueryRow(ctx, `
		INSERT INTO idempotency_records (id, idempotency_key, request_hash, response_status, response_body)
		VALUES ($1, $2, $3, 0, '{}')
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id, idempotency_key, request_hash, transfer_id, response_status, response_body`,
		domain.NewID("idm"), key, hash,
	).Scan(&record.ID, &record.Key, &record.RequestHash, &record.TransferID, &record.ResponseStatus, &record.ResponseBody)
	if errors.Is(err, pgx.ErrNoRows) {
		existing, err := getIdempotency(ctx, tx, key)
		if err != nil {
			return false, IdempotencyRecord{}, err
		}
		return false, existing, nil
	}
	if err != nil {
		return false, IdempotencyRecord{}, fmt.Errorf("claim idempotency key: %w", err)
	}
	return true, record, nil
}

func (s *Store) CompleteIdempotency(ctx context.Context, tx pgx.Tx, key, transferID string, status int, body string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE idempotency_records
		SET transfer_id = $2, response_status = $3, response_body = $4
		WHERE idempotency_key = $1 AND response_status = 0`,
		key, transferID, status, body,
	)
	if err != nil {
		return fmt.Errorf("complete idempotency key: %w", err)
	}
	return mustAffectOne(tag, "complete idempotency key")
}

func getIdempotency(ctx context.Context, tx pgx.Tx, key string) (IdempotencyRecord, error) {
	var record IdempotencyRecord
	err := tx.QueryRow(ctx, `
		SELECT id, idempotency_key, request_hash, transfer_id, response_status, response_body
		FROM idempotency_records
		WHERE idempotency_key = $1`, key,
	).Scan(&record.ID, &record.Key, &record.RequestHash, &record.TransferID, &record.ResponseStatus, &record.ResponseBody)
	if err != nil {
		return IdempotencyRecord{}, fmt.Errorf("load idempotency key: %w", err)
	}
	return record, nil
}
