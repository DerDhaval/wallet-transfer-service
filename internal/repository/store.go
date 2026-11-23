package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

// WithTx runs fn in READ COMMITTED. A returned error rolls the transaction back.
func (s *Store) WithTx(ctx context.Context, fn func(ctx context.Context, tx pgx.Tx) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SET LOCAL lock_timeout = '5s'`); err != nil {
		return fmt.Errorf("set lock timeout: %w", err)
	}
	if _, err := tx.Exec(ctx, `SET LOCAL statement_timeout = '15s'`); err != nil {
		return fmt.Errorf("set statement timeout: %w", err)
	}
	if err := fn(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

func mustAffectOne(tag pgconn.CommandTag, what string) error {
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%s: expected exactly one row to change", what)
	}
	return nil
}
