package wallet

import (
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
)

// OpenParams carries the input required to open a wallet.
type OpenParams struct {
	WalletID       string
	PlayerID       string
	InitialBalance money.Money
	OpeningTxID    string // required when InitialBalance > 0
	LedgerEntryID  string // required when InitialBalance > 0
	Now            time.Time
}

// OpenResult is the outcome of opening a wallet.
type OpenResult struct {
	Wallet Wallet
	Ledger *LedgerEntry // nil when initial balance is zero
}

// MovementResult is the outcome of a credit or debit.
type MovementResult struct {
	Wallet Wallet
	Ledger LedgerEntry
}
