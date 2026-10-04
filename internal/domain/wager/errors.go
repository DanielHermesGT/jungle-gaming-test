package wager

import "errors"

var (
	ErrInvalidKind       = errors.New("wager: invalid kind")
	ErrInvalidStatus     = errors.New("wager: invalid status")
	ErrInvalidOrigin     = errors.New("wager: invalid origin")
	ErrInvalidInput      = errors.New("wager: invalid input")
	ErrInvalidAmount     = errors.New("wager: invalid amount")
	ErrInvalidTransition = errors.New("wager: invalid transition")
	ErrTerminalStatus    = errors.New("wager: terminal status")
	ErrMissingReference  = errors.New("wager: missing reference")
	ErrUninitialized     = errors.New("wager: uninitialized")
)
