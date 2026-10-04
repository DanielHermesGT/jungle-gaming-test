package wager

// Kind is the wager transaction type (README §6.3 / §7).
type Kind string

const (
	KindOpening  Kind = "OPENING"
	KindBet      Kind = "BET"
	KindWin      Kind = "WIN"
	KindLoss     Kind = "LOSS"
	KindRefund   Kind = "REFUND"
	KindRollback Kind = "ROLLBACK"
)

// ParseKind validates a kind string.
func ParseKind(s string) (Kind, error) {
	k := Kind(s)
	switch k {
	case KindOpening, KindBet, KindWin, KindLoss, KindRefund, KindRollback:
		return k, nil
	default:
		return "", ErrInvalidKind
	}
}

// IsExternal reports whether the kind is allowed for HTTP/SQS (not OPENING).
func (k Kind) IsExternal() bool {
	switch k {
	case KindBet, KindWin, KindLoss, KindRefund, KindRollback:
		return true
	default:
		return false
	}
}

// RequiresReference reports whether referenceExternalTransactionId is mandatory.
func (k Kind) RequiresReference() bool {
	return k == KindRefund || k == KindRollback
}
