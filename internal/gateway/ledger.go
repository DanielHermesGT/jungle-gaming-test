package gateway

import (
	"context"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wallet"
)

// LedgerRepository persists append-only wallet ledger entries.
type LedgerRepository interface {
	Insert(ctx context.Context, q Querier, entry wallet.LedgerEntry) error
	ListByWalletID(ctx context.Context, q Querier, walletID string) ([]wallet.LedgerEntry, error)
	ListByWalletIDPage(ctx context.Context, q Querier, walletID string, cursorCreatedAt time.Time, cursorID string, limit int) ([]wallet.LedgerEntry, error)
}
