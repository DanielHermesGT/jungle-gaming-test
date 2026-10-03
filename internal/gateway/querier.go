package gateway

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Querier is implemented by *pgxpool.Pool and pgx.Tx.
type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// TxRunner runs work inside a SQL transaction.
type TxRunner interface {
	WithinTx(ctx context.Context, fn func(q Querier) error) error
}

// ReadQuerier exposes a non-transactional Querier (connection pool).
type ReadQuerier interface {
	Querier() Querier
}

// DB combines transactional and non-transactional access for use cases.
type DB interface {
	TxRunner
	ReadQuerier
}
