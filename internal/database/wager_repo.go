package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wager"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/gateway"
)

const wagerSelectCols = `
id, origin, kind, status, wallet_id, player_id, amount_minor, currency,
created_at, updated_at, failure_code, result_balance_minor,
provider_id, external_transaction_id, idempotency_key, payload_hash,
round_id, game_id, reference_external_transaction_id, resolved_reference_id,
pending_reference_until`

type WagerRepo struct{}

func NewWagerRepo() *WagerRepo {
	return &WagerRepo{}
}

var _ gateway.WagerRepository = (*WagerRepo)(nil)

func (r *WagerRepo) Insert(ctx context.Context, q gateway.Querier, tx wager.Transaction) error {
	const sql = `
INSERT INTO wager_transactions (
    id, origin, kind, status, wallet_id, player_id, amount_minor, currency,
    created_at, updated_at, failure_code, result_balance_minor,
    provider_id, external_transaction_id, idempotency_key, payload_hash,
    round_id, game_id, reference_external_transaction_id, resolved_reference_id,
    pending_reference_until
) VALUES (
    $1,$2,$3,$4,$5,$6,$7,$8,
    $9,$10,$11,$12,
    $13,$14,$15,$16,
    $17,$18,$19,$20,
    $21
)`

	_, err := q.Exec(ctx, sql,
		tx.ID(),
		string(tx.Origin()),
		string(tx.Kind()),
		string(tx.Status()),
		tx.WalletID(),
		tx.PlayerID(),
		tx.Amount().Minor(),
		tx.Amount().Currency(),
		tx.CreatedAt(),
		tx.UpdatedAt(),
		nullStr(string(tx.FailureCode())),
		nullInt64(tx.ResultBalance()),
		nullStr(tx.ProviderID()),
		nullStr(tx.ExternalTransactionID()),
		nullStr(tx.IdempotencyKey()),
		nullStr(tx.PayloadHash()),
		nullStr(tx.RoundID()),
		nullStr(tx.GameID()),
		nullStr(tx.ReferenceExternalTransaction()),
		nullStr(tx.ResolvedReferenceID()),
		nullTime(tx.PendingReferenceUntil()),
	)
	if isUniqueViolation(err) {
		return fmt.Errorf("%w: wager unique", ErrConflict)
	}
	if err != nil {
		return fmt.Errorf("database: insert wager: %w", err)
	}
	return nil
}

func (r *WagerRepo) Update(ctx context.Context, q gateway.Querier, tx wager.Transaction) error {
	const sql = `
UPDATE wager_transactions
SET status = $2,
    updated_at = $3,
    failure_code = $4,
    result_balance_minor = $5,
    resolved_reference_id = $6,
    pending_reference_until = $7
WHERE id = $1`

	tag, err := q.Exec(ctx, sql,
		tx.ID(),
		string(tx.Status()),
		tx.UpdatedAt(),
		nullStr(string(tx.FailureCode())),
		nullInt64(tx.ResultBalance()),
		nullStr(tx.ResolvedReferenceID()),
		nullTime(tx.PendingReferenceUntil()),
	)
	if err != nil {
		return fmt.Errorf("database: update wager: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *WagerRepo) GetByID(ctx context.Context, q gateway.Querier, id string) (wager.Transaction, error) {
	sql := `SELECT ` + wagerSelectCols + ` FROM wager_transactions WHERE id = $1`
	return r.scanOne(ctx, q, sql, id)
}

func (r *WagerRepo) GetByIDForUpdate(ctx context.Context, q gateway.Querier, id string) (wager.Transaction, error) {
	sql := `SELECT ` + wagerSelectCols + ` FROM wager_transactions WHERE id = $1 FOR UPDATE`
	return r.scanOne(ctx, q, sql, id)
}

func (r *WagerRepo) GetByIdempotencyKey(ctx context.Context, q gateway.Querier, key string) (wager.Transaction, error) {
	sql := `SELECT ` + wagerSelectCols + ` FROM wager_transactions WHERE idempotency_key = $1`
	return r.scanOne(ctx, q, sql, key)
}

func (r *WagerRepo) GetByProviderExternal(ctx context.Context, q gateway.Querier, providerID, externalTxID string) (wager.Transaction, error) {
	sql := `SELECT ` + wagerSelectCols + `
FROM wager_transactions
WHERE provider_id = $1 AND external_transaction_id = $2`
	return r.scanOne(ctx, q, sql, providerID, externalTxID)
}

func (r *WagerRepo) GetProcessedReversal(
	ctx context.Context,
	q gateway.Querier,
	providerID, referenceExternalID string,
	kinds ...wager.Kind,
) (wager.Transaction, error) {
	if len(kinds) == 0 {
		kinds = []wager.Kind{wager.KindRefund, wager.KindRollback}
	}
	kindStrs := make([]string, len(kinds))
	for i, k := range kinds {
		kindStrs[i] = string(k)
	}
	sql := `SELECT ` + wagerSelectCols + `
FROM wager_transactions
WHERE provider_id = $1
  AND reference_external_transaction_id = $2
  AND kind = ANY($3)
  AND status = 'PROCESSED'
LIMIT 1`
	return r.scanOne(ctx, q, sql, providerID, referenceExternalID, kindStrs)
}

func (r *WagerRepo) ListPendingReferenceDue(
	ctx context.Context,
	q gateway.Querier,
	now time.Time,
	limit int,
) ([]wager.Transaction, error) {
	if limit < 1 {
		limit = 50
	}
	sql := `SELECT ` + wagerSelectCols + `
FROM wager_transactions
WHERE status = 'PENDING_REFERENCE'
  AND pending_reference_until IS NOT NULL
  AND pending_reference_until <= $1
ORDER BY pending_reference_until ASC, id ASC
LIMIT $2`

	return r.scanWagerList(ctx, q, sql, now.UTC(), limit)
}

func (r *WagerRepo) ListPendingReference(
	ctx context.Context,
	q gateway.Querier,
	limit int,
) ([]wager.Transaction, error) {
	if limit < 1 {
		limit = 50
	}
	sql := `SELECT ` + wagerSelectCols + `
FROM wager_transactions
WHERE status = 'PENDING_REFERENCE'
ORDER BY pending_reference_until ASC NULLS LAST, id ASC
LIMIT $1`

	return r.scanWagerList(ctx, q, sql, limit)
}

func (r *WagerRepo) scanWagerList(
	ctx context.Context,
	q gateway.Querier,
	sql string,
	args ...any,
) ([]wager.Transaction, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("database: list wager: %w", err)
	}
	defer rows.Close()

	out := make([]wager.Transaction, 0)
	for rows.Next() {
		row, err := scanWagerRow(rows)
		if err != nil {
			return nil, err
		}
		tx, err := row.toDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, tx)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("database: list wager: %w", err)
	}
	return out, nil
}

func (r *WagerRepo) scanOne(ctx context.Context, q gateway.Querier, sql string, args ...any) (wager.Transaction, error) {
	row := q.QueryRow(ctx, sql, args...)
	wr, err := scanWagerRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return wager.Transaction{}, ErrNotFound
	}
	if err != nil {
		return wager.Transaction{}, fmt.Errorf("database: get wager: %w", err)
	}
	return wr.toDomain()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanWagerRow(s scannable) (wagerRow, error) {
	var row wagerRow
	err := s.Scan(
		&row.id,
		&row.origin,
		&row.kind,
		&row.status,
		&row.walletID,
		&row.playerID,
		&row.amountMinor,
		&row.currency,
		&row.createdAt,
		&row.updatedAt,
		&row.failureCode,
		&row.resultBalanceMinor,
		&row.providerID,
		&row.externalTransactionID,
		&row.idempotencyKey,
		&row.payloadHash,
		&row.roundID,
		&row.gameID,
		&row.referenceExternalTransaction,
		&row.resolvedReferenceID,
		&row.pendingReferenceUntil,
	)
	return row, err
}
