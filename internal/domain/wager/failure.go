package wager

// FailureCode is a stable rejection/failure code (README §7).
type FailureCode string

const (
	FailureInsufficientFunds         FailureCode = "INSUFFICIENT_FUNDS"
	FailureReversalInsufficientFunds FailureCode = "REVERSAL_INSUFFICIENT_FUNDS"
	FailureReferenceNotFound         FailureCode = "REFERENCE_NOT_FOUND"
	FailureReferenceNotProcessed     FailureCode = "REFERENCE_NOT_PROCESSED"
	FailureDuplicateReversal         FailureCode = "DUPLICATE_REVERSAL"
	FailureInvalidAmount             FailureCode = "INVALID_AMOUNT"
	FailureInvalidKind               FailureCode = "INVALID_KIND"
	FailureInvalidTransition         FailureCode = "INVALID_TRANSITION"
)

// Valid reports whether the code is a known non-empty failure code.
func (c FailureCode) Valid() bool {
	switch c {
	case FailureInsufficientFunds,
		FailureReversalInsufficientFunds,
		FailureReferenceNotFound,
		FailureReferenceNotProcessed,
		FailureDuplicateReversal,
		FailureInvalidAmount,
		FailureInvalidKind,
		FailureInvalidTransition:
		return true
	default:
		return false
	}
}
