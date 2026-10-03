package postgres

import (
	"context"
	"fmt"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wallet"
)

// LedgerRepo persists wallet ledger entries.
//
// Append-only — proibido Update/Delete de lançamentos (README §6.4).
// TODO(futuro): callers devem incluir inbox/outbox/wager na mesma Tx que Insert ledger.
type LedgerRepo struct{}

func NewLedgerRepo() *LedgerRepo {
	return &LedgerRepo{}
}

func (r *LedgerRepo) Insert(ctx context.Context, q Querier, entry wallet.LedgerEntry) error {
	const sql = `
INSERT INTO wallet_ledger_entries (
    id, wallet_id, transaction_id, direction,
    amount_minor, currency, balance_before_minor, balance_after_minor, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err := q.Exec(ctx, sql,
		entry.ID(),
		entry.WalletID(),
		entry.TransactionID(),
		string(entry.Direction()),
		entry.Amount().Minor(),
		entry.Amount().Currency(),
		entry.BalanceBefore().Minor(),
		entry.BalanceAfter().Minor(),
		entry.CreatedAt(),
	)
	if isUniqueViolation(err) {
		return fmt.Errorf("%w: ledger wallet/transaction", ErrConflict)
	}
	if err != nil {
		return fmt.Errorf("postgres: insert ledger: %w", err)
	}
	return nil
}

func (r *LedgerRepo) ListByWalletID(ctx context.Context, q Querier, walletID string) ([]wallet.LedgerEntry, error) {
	const sql = `
SELECT id, wallet_id, transaction_id, direction,
       amount_minor, currency, balance_before_minor, balance_after_minor, created_at
FROM wallet_ledger_entries
WHERE wallet_id = $1
ORDER BY created_at ASC, id ASC`

	rows, err := q.Query(ctx, sql, walletID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list ledger: %w", err)
	}
	defer rows.Close()

	var out []wallet.LedgerEntry
	for rows.Next() {
		var row ledgerRow
		if err := rows.Scan(
			&row.id,
			&row.walletID,
			&row.transactionID,
			&row.direction,
			&row.amountMinor,
			&row.currency,
			&row.balanceBeforeMinor,
			&row.balanceAfterMinor,
			&row.createdAt,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan ledger: %w", err)
		}
		row.currency = trimCurrency(row.currency)
		entry, err := row.toDomain()
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list ledger rows: %w", err)
	}
	return out, nil
}
