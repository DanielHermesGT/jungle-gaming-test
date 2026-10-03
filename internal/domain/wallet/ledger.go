package wallet

import (
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
)

type Direction string

const (
	DirectionDebit  Direction = "DEBIT"
	DirectionCredit Direction = "CREDIT"
)

// LedgerEntry is an immutable wallet ledger posting.
type LedgerEntry struct {
	id            string
	walletID      string
	transactionID string
	direction     Direction
	amount        money.Money
	balanceBefore money.Money
	balanceAfter  money.Money
	createdAt     time.Time
}

func (e LedgerEntry) ID() string                 { return e.id }
func (e LedgerEntry) WalletID() string           { return e.walletID }
func (e LedgerEntry) TransactionID() string      { return e.transactionID }
func (e LedgerEntry) Direction() Direction       { return e.direction }
func (e LedgerEntry) Amount() money.Money        { return e.amount }
func (e LedgerEntry) BalanceBefore() money.Money { return e.balanceBefore }
func (e LedgerEntry) BalanceAfter() money.Money  { return e.balanceAfter }
func (e LedgerEntry) CreatedAt() time.Time       { return e.createdAt }

func newLedgerEntry(
	id, walletID, transactionID string,
	direction Direction,
	amount, balanceBefore, balanceAfter money.Money,
	createdAt time.Time,
) (LedgerEntry, error) {
	if id == "" || walletID == "" || transactionID == "" {
		return LedgerEntry{}, ErrInvalidLedger
	}
	if direction != DirectionDebit && direction != DirectionCredit {
		return LedgerEntry{}, ErrInvalidLedger
	}
	if !moneyInitialized(amount) || amount.IsZero() || amount.IsNegative() {
		return LedgerEntry{}, ErrInvalidLedger
	}
	if !moneyInitialized(balanceBefore) || !moneyInitialized(balanceAfter) {
		return LedgerEntry{}, ErrInvalidLedger
	}
	if balanceBefore.IsNegative() || balanceAfter.IsNegative() {
		return LedgerEntry{}, ErrInvalidLedger
	}

	var expected money.Money
	var err error
	switch direction {
	case DirectionCredit:
		expected, err = balanceBefore.Add(amount)
	case DirectionDebit:
		expected, err = balanceBefore.Sub(amount)
	}
	if err != nil {
		return LedgerEntry{}, err
	}
	if !expected.Equal(balanceAfter) {
		return LedgerEntry{}, ErrInvalidLedger
	}

	return LedgerEntry{
		id:            id,
		walletID:      walletID,
		transactionID: transactionID,
		direction:     direction,
		amount:        amount,
		balanceBefore: balanceBefore,
		balanceAfter:  balanceAfter,
		createdAt:     createdAt,
	}, nil
}

func moneyInitialized(m money.Money) bool {
	return m.Currency() != ""
}
