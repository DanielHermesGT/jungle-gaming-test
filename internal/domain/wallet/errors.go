package wallet

import "errors"

var (
	ErrInvalidWallet     = errors.New("wallet: invalid wallet")
	ErrInvalidPlayer     = errors.New("wallet: invalid player")
	ErrInsufficientFunds = errors.New("wallet: insufficient funds")
	ErrUninitialized     = errors.New("wallet: uninitialized")
	ErrInvalidMovement   = errors.New("wallet: invalid movement")
	ErrInvalidLedger     = errors.New("wallet: invalid ledger entry")
)
