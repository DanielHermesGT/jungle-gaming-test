package database

import (
	"fmt"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wager"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wallet"
)

type walletRow struct {
	id           string
	playerID     string
	currency     string
	balanceMinor int64
	version      int64
	createdAt    time.Time
	updatedAt    time.Time
}

func (r walletRow) toDomain() (wallet.Wallet, error) {
	bal, err := money.FromMinor(r.balanceMinor, r.currency)
	if err != nil {
		return wallet.Wallet{}, fmt.Errorf("postgres: wallet money: %w", err)
	}
	return wallet.WalletFromPersisted(r.id, r.playerID, bal, r.version, r.createdAt, r.updatedAt)
}

type ledgerRow struct {
	id                 string
	walletID           string
	transactionID      string
	direction          string
	amountMinor        int64
	currency           string
	balanceBeforeMinor int64
	balanceAfterMinor  int64
	createdAt          time.Time
}

func (r ledgerRow) toDomain() (wallet.LedgerEntry, error) {
	amount, err := money.FromMinor(r.amountMinor, r.currency)
	if err != nil {
		return wallet.LedgerEntry{}, fmt.Errorf("postgres: ledger amount: %w", err)
	}
	before, err := money.FromMinor(r.balanceBeforeMinor, r.currency)
	if err != nil {
		return wallet.LedgerEntry{}, fmt.Errorf("postgres: ledger before: %w", err)
	}
	after, err := money.FromMinor(r.balanceAfterMinor, r.currency)
	if err != nil {
		return wallet.LedgerEntry{}, fmt.Errorf("postgres: ledger after: %w", err)
	}
	return wallet.LedgerEntryFromPersisted(
		r.id,
		r.walletID,
		r.transactionID,
		wallet.Direction(r.direction),
		amount,
		before,
		after,
		r.createdAt,
	)
}

type wagerRow struct {
	id                           string
	origin                       string
	kind                         string
	status                       string
	walletID                     string
	playerID                     string
	amountMinor                  int64
	currency                     string
	createdAt                    time.Time
	updatedAt                    time.Time
	failureCode                  *string
	resultBalanceMinor           *int64
	providerID                   *string
	externalTransactionID        *string
	idempotencyKey               *string
	payloadHash                  *string
	roundID                      *string
	gameID                       *string
	referenceExternalTransaction *string
	resolvedReferenceID          *string
	pendingReferenceUntil        *time.Time
}

func (r wagerRow) toDomain() (wager.Transaction, error) {
	amount, err := money.FromMinor(r.amountMinor, trimCurrency(r.currency))
	if err != nil {
		return wager.Transaction{}, fmt.Errorf("database: wager amount: %w", err)
	}
	var resultBal *money.Money
	if r.resultBalanceMinor != nil {
		bal, err := money.FromMinor(*r.resultBalanceMinor, trimCurrency(r.currency))
		if err != nil {
			return wager.Transaction{}, fmt.Errorf("database: wager result balance: %w", err)
		}
		resultBal = &bal
	}
	var until time.Time
	if r.pendingReferenceUntil != nil {
		until = r.pendingReferenceUntil.UTC()
	}
	return wager.FromPersisted(wager.PersistedParams{
		ID:                           r.id,
		Origin:                       wager.Origin(r.origin),
		Kind:                         wager.Kind(r.kind),
		Status:                       wager.Status(r.status),
		WalletID:                     r.walletID,
		PlayerID:                     r.playerID,
		Amount:                       amount,
		CreatedAt:                    r.createdAt,
		UpdatedAt:                    r.updatedAt,
		FailureCode:                  wager.FailureCode(derefStr(r.failureCode)),
		ResultBalance:                resultBal,
		ProviderID:                   derefStr(r.providerID),
		ExternalTransactionID:        derefStr(r.externalTransactionID),
		IdempotencyKey:               derefStr(r.idempotencyKey),
		PayloadHash:                  derefStr(r.payloadHash),
		RoundID:                      derefStr(r.roundID),
		GameID:                       derefStr(r.gameID),
		ReferenceExternalTransaction: derefStr(r.referenceExternalTransaction),
		ResolvedReferenceID:          derefStr(r.resolvedReferenceID),
		PendingReferenceUntil:        until,
	})
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	u := t.UTC()
	return &u
}

func nullInt64(m *money.Money) *int64 {
	if m == nil {
		return nil
	}
	v := m.Minor()
	return &v
}
