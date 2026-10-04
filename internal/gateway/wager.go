package gateway

import (
	"context"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wager"
)

// WagerRepository persiste transações de wagering.
type WagerRepository interface {
	// Insert grava uma nova transação (estado final da TX ou PENDING_REFERENCE).
	Insert(ctx context.Context, q Querier, tx wager.Transaction) error
	// Update atualiza status, failure code, saldo resultado, ref resolvida e TTL.
	Update(ctx context.Context, q Querier, tx wager.Transaction) error
	// GetByID busca pelo id interno.
	GetByID(ctx context.Context, q Querier, id string) (wager.Transaction, error)
	// GetByIDForUpdate busca pelo id interno com lock FOR UPDATE.
	GetByIDForUpdate(ctx context.Context, q Querier, id string) (wager.Transaction, error)
	// GetByIdempotencyKey busca pela chave de idempotência (replay/conflito).
	GetByIdempotencyKey(ctx context.Context, q Querier, key string) (wager.Transaction, error)
	// GetByProviderExternal busca por (providerId, externalTransactionId).
	GetByProviderExternal(ctx context.Context, q Querier, providerID, externalTxID string) (wager.Transaction, error)
	// GetProcessedReversal busca reversão PROCESSED do mesmo kind para a referência (anti-duplicata).
	GetProcessedReversal(ctx context.Context, q Querier, providerID, referenceExternalID string, kind wager.Kind) (wager.Transaction, error)
	// ListPendingReferenceDue lista PENDING_REFERENCE com TTL vencido (worker futuro).
	ListPendingReferenceDue(ctx context.Context, q Querier, now time.Time, limit int) ([]wager.Transaction, error)
}
