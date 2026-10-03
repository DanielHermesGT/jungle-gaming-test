package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/gateway"
)

// DB wraps a connection pool.
type DB struct {
	Pool *pgxpool.Pool
}

func NewDB(ctx context.Context, databaseURL string) (*DB, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("database: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: ping: %w", err)
	}
	return &DB{Pool: pool}, nil
}

func (db *DB) Close() {
	if db != nil && db.Pool != nil {
		db.Pool.Close()
	}
}

// Querier returns the pool for reads outside an explicit transaction.
func (db *DB) Querier() gateway.Querier {
	return db.Pool
}

// WithinTx runs fn inside a transaction.
//
// Alterações de saldo devem persistir wallet + ledger no mesmo Commit.
// TODO(futuro): estender a mesma Tx para wager_transaction / inbox / outbox.
func (db *DB) WithinTx(ctx context.Context, fn func(q gateway.Querier) error) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("database: begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("database: commit: %w", err)
	}
	return nil
}

var (
	_ gateway.TxRunner    = (*DB)(nil)
	_ gateway.ReadQuerier = (*DB)(nil)
)

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
