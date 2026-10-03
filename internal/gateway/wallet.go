package gateway

import (
	"context"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wallet"
)

// WalletRepository persists wallet aggregate state.
type WalletRepository interface {
	Insert(ctx context.Context, q Querier, w wallet.Wallet) error
	GetByID(ctx context.Context, q Querier, id string) (wallet.Wallet, error)
	GetByIDForUpdate(ctx context.Context, q Querier, id string) (wallet.Wallet, error)
	Update(ctx context.Context, q Querier, w wallet.Wallet) error
}
