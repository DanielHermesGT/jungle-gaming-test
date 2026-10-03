package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wallet"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/gateway"
)

type WalletRepo struct{}

func NewWalletRepo() *WalletRepo {
	return &WalletRepo{}
}

var _ gateway.WalletRepository = (*WalletRepo)(nil)

func (r *WalletRepo) Insert(ctx context.Context, q gateway.Querier, w wallet.Wallet) error {
	const sql = `
INSERT INTO wallets (id, player_id, currency, balance_minor, version, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err := q.Exec(ctx, sql,
		w.ID(),
		w.PlayerID(),
		w.Balance().Currency(),
		w.Balance().Minor(),
		w.Version(),
		w.CreatedAt(),
		w.UpdatedAt(),
	)
	if isUniqueViolation(err) {
		return fmt.Errorf("%w: wallet player/currency", ErrConflict)
	}
	if err != nil {
		return fmt.Errorf("database: insert wallet: %w", err)
	}
	return nil
}

func (r *WalletRepo) GetByID(ctx context.Context, q gateway.Querier, id string) (wallet.Wallet, error) {
	const sql = `
SELECT id, player_id, currency, balance_minor, version, created_at, updated_at
FROM wallets
WHERE id = $1`
	return r.scanOne(ctx, q, sql, id)
}

// GetByIDForUpdate locks the wallet row for the current transaction (pessimistic concurrency).
func (r *WalletRepo) GetByIDForUpdate(ctx context.Context, q gateway.Querier, id string) (wallet.Wallet, error) {
	const sql = `
SELECT id, player_id, currency, balance_minor, version, created_at, updated_at
FROM wallets
WHERE id = $1
FOR UPDATE`
	return r.scanOne(ctx, q, sql, id)
}

func (r *WalletRepo) Update(ctx context.Context, q gateway.Querier, w wallet.Wallet) error {
	const sql = `
UPDATE wallets
SET balance_minor = $2,
    version = $3,
    updated_at = $4
WHERE id = $1`

	tag, err := q.Exec(ctx, sql,
		w.ID(),
		w.Balance().Minor(),
		w.Version(),
		w.UpdatedAt(),
	)
	if err != nil {
		return fmt.Errorf("database: update wallet: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *WalletRepo) scanOne(ctx context.Context, q gateway.Querier, sql string, id string) (wallet.Wallet, error) {
	var row walletRow
	err := q.QueryRow(ctx, sql, id).Scan(
		&row.id,
		&row.playerID,
		&row.currency,
		&row.balanceMinor,
		&row.version,
		&row.createdAt,
		&row.updatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return wallet.Wallet{}, ErrNotFound
	}
	if err != nil {
		return wallet.Wallet{}, fmt.Errorf("database: get wallet: %w", err)
	}
	// CHAR(3) may come padded; trim for domain validation.
	row.currency = trimCurrency(row.currency)
	return row.toDomain()
}

func trimCurrency(c string) string {
	for len(c) > 0 && c[len(c)-1] == ' ' {
		c = c[:len(c)-1]
	}
	return c
}
