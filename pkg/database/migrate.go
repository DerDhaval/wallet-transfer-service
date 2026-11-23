package database

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"wallet-transfer-service/migrations"
)

// Migrate applies every pending *.up.sql migration in lexical order.
// Version names look like 000001_create_wallets (golang-migrate style).
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if err := ensureMigrationTable(ctx, pool); err != nil {
		return err
	}

	versions, err := listUpVersions()
	if err != nil {
		return err
	}
	for _, version := range versions {
		var applied bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if applied {
			continue
		}
		body, err := migrations.FS.ReadFile(version + ".up.sql")
		if err != nil {
			return fmt.Errorf("read migration %s: %w", version, err)
		}
		if err := apply(ctx, pool, version, string(body), true); err != nil {
			return err
		}
	}
	return nil
}

// MigrateDown rolls back the latest applied migration using its *.down.sql file.
func MigrateDown(ctx context.Context, pool *pgxpool.Pool) error {
	if err := ensureMigrationTable(ctx, pool); err != nil {
		return err
	}

	var version string
	err := pool.QueryRow(ctx, `
		SELECT version
		FROM schema_migrations
		ORDER BY version DESC
		LIMIT 1`).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load latest migration: %w", err)
	}

	body, err := migrations.FS.ReadFile(version + ".down.sql")
	if err != nil {
		return fmt.Errorf("read down migration %s: %w", version, err)
	}
	return apply(ctx, pool, version, string(body), false)
}

func ensureMigrationTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	return nil
}

func listUpVersions() ([]string, error) {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	versions := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		versions = append(versions, strings.TrimSuffix(name, ".up.sql"))
	}
	sort.Strings(versions)
	return versions, nil
}

func apply(ctx context.Context, pool *pgxpool.Pool, version, sql string, up bool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", version, err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := execScript(ctx, tx, sql); err != nil {
		direction := "up"
		if !up {
			direction = "down"
		}
		return fmt.Errorf("apply %s migration %s: %w", direction, version, err)
	}
	if up {
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
			return fmt.Errorf("record migration %s: %w", version, err)
		}
	} else {
		if _, err := tx.Exec(ctx, `DELETE FROM schema_migrations WHERE version = $1`, version); err != nil {
			return fmt.Errorf("unrecord migration %s: %w", version, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %s: %w", version, err)
	}
	action := "migration applied:"
	if !up {
		action = "migration rolled back:"
	}
	fmt.Println(action, version)
	return nil
}

func execScript(ctx context.Context, tx pgx.Tx, sql string) error {
	reader := tx.Conn().PgConn().Exec(ctx, sql)
	if _, err := reader.ReadAll(); err != nil {
		_ = reader.Close()
		return err
	}
	return reader.Close()
}
