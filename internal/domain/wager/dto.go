package wager

import (
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
)

// OpeningParams creates an internal OPENING transaction (README §6.3 / §9).
type OpeningParams struct {
	ID       string
	WalletID string
	PlayerID string
	Amount   money.Money
	Now      time.Time
}

// ExternalParams creates a provider-facing wager transaction in PENDING.
type ExternalParams struct {
	ID                           string
	Kind                         Kind
	WalletID                     string
	PlayerID                     string
	ProviderID                   string
	ExternalTransactionID        string
	IdempotencyKey               string
	PayloadHash                  string
	RoundID                      string
	GameID                       string
	Amount                       money.Money
	ReferenceExternalTransaction string // required for REFUND / ROLLBACK
	Now                          time.Time
}
