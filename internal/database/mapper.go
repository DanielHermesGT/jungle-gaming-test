package database

import (
	"fmt"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
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
