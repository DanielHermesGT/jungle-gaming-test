package wallet

import (
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
)

// Wallet is the financial aggregate root.
type Wallet struct {
	id        string
	playerID  string
	balance   money.Money
	version   int64
	createdAt time.Time
	updatedAt time.Time
}

// Open creates a wallet. Positive initial balance produces one CREDIT ledger entry.
// Zero initial balance creates the wallet without a ledger entry.
func Open(p OpenParams) (OpenResult, error) {
	if p.WalletID == "" {
		return OpenResult{}, ErrInvalidWallet
	}
	if p.PlayerID == "" {
		return OpenResult{}, ErrInvalidPlayer
	}
	if !moneyInitialized(p.InitialBalance) {
		return OpenResult{}, ErrInvalidMovement
	}
	if p.InitialBalance.IsNegative() {
		return OpenResult{}, ErrInvalidMovement
	}

	w := Wallet{
		id:        p.WalletID,
		playerID:  p.PlayerID,
		balance:   p.InitialBalance,
		version:   1,
		createdAt: p.Now,
		updatedAt: p.Now,
	}

	if p.InitialBalance.IsZero() {
		return OpenResult{Wallet: w, Ledger: nil}, nil
	}

	if p.OpeningTxID == "" || p.LedgerEntryID == "" {
		return OpenResult{}, ErrInvalidMovement
	}

	zero, err := money.Zero(p.InitialBalance.Currency())
	if err != nil {
		return OpenResult{}, err
	}
	entry, err := newLedgerEntry(
		p.LedgerEntryID,
		p.WalletID,
		p.OpeningTxID,
		DirectionCredit,
		p.InitialBalance,
		zero,
		p.InitialBalance,
		p.Now,
	)
	if err != nil {
		return OpenResult{}, err
	}
	return OpenResult{Wallet: w, Ledger: &entry}, nil
}

// WalletFromPersisted rebuilds a wallet from persisted state without reapplying movements.
func WalletFromPersisted(id, playerID string, balance money.Money, version int64, createdAt, updatedAt time.Time) (Wallet, error) {
	if id == "" {
		return Wallet{}, ErrInvalidWallet
	}
	if playerID == "" {
		return Wallet{}, ErrInvalidPlayer
	}
	if !moneyInitialized(balance) || balance.IsNegative() {
		return Wallet{}, ErrInvalidMovement
	}
	if version < 1 {
		return Wallet{}, ErrInvalidWallet
	}
	return Wallet{
		id:        id,
		playerID:  playerID,
		balance:   balance,
		version:   version,
		createdAt: createdAt,
		updatedAt: updatedAt,
	}, nil
}

func (w Wallet) ID() string           { return w.id }
func (w Wallet) PlayerID() string     { return w.playerID }
func (w Wallet) Balance() money.Money { return w.balance }
func (w Wallet) Version() int64       { return w.version }
func (w Wallet) CreatedAt() time.Time { return w.createdAt }
func (w Wallet) UpdatedAt() time.Time { return w.updatedAt }

func (w Wallet) valid() bool {
	return w.id != "" && w.playerID != "" && moneyInitialized(w.balance) && w.version >= 1
}

// Credit increases the balance and returns the corresponding CREDIT ledger entry.
func (w Wallet) Credit(ledgerEntryID, transactionID string, amount money.Money, now time.Time) (MovementResult, error) {
	return w.apply(DirectionCredit, ledgerEntryID, transactionID, amount, now)
}

// Debit decreases the balance and returns the corresponding DEBIT ledger entry.
func (w Wallet) Debit(ledgerEntryID, transactionID string, amount money.Money, now time.Time) (MovementResult, error) {
	return w.apply(DirectionDebit, ledgerEntryID, transactionID, amount, now)
}

func (w Wallet) apply(dir Direction, ledgerEntryID, transactionID string, amount money.Money, now time.Time) (MovementResult, error) {
	if !w.valid() {
		return MovementResult{}, ErrUninitialized
	}
	if ledgerEntryID == "" || transactionID == "" {
		return MovementResult{}, ErrInvalidMovement
	}
	if !moneyInitialized(amount) || amount.IsZero() || amount.IsNegative() {
		return MovementResult{}, ErrInvalidMovement
	}
	if amount.Currency() != w.balance.Currency() {
		return MovementResult{}, money.ErrCurrencyMismatch
	}

	var after money.Money
	var err error
	switch dir {
	case DirectionCredit:
		after, err = w.balance.Add(amount)
	case DirectionDebit:
		cmp, cmpErr := w.balance.Cmp(amount)
		if cmpErr != nil {
			return MovementResult{}, cmpErr
		}
		if cmp < 0 {
			return MovementResult{}, ErrInsufficientFunds
		}
		after, err = w.balance.Sub(amount)
	default:
		return MovementResult{}, ErrInvalidMovement
	}
	if err != nil {
		return MovementResult{}, err
	}

	entry, err := newLedgerEntry(ledgerEntryID, w.id, transactionID, dir, amount, w.balance, after, now)
	if err != nil {
		return MovementResult{}, err
	}

	next := Wallet{
		id:        w.id,
		playerID:  w.playerID,
		balance:   after,
		version:   w.version + 1,
		createdAt: w.createdAt,
		updatedAt: now,
	}
	return MovementResult{Wallet: next, Ledger: entry}, nil
}
