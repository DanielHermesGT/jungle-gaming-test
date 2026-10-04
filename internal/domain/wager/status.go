package wager

// Status is the wager transaction lifecycle state (README §6.3).
type Status string

const (
	StatusPending          Status = "PENDING"
	StatusPendingReference Status = "PENDING_REFERENCE"
	StatusProcessed        Status = "PROCESSED"
	StatusRejected         Status = "REJECTED"
	StatusFailed           Status = "FAILED"
)

// ParseStatus validates a status string.
func ParseStatus(s string) (Status, error) {
	st := Status(s)
	switch st {
	case StatusPending, StatusPendingReference, StatusProcessed, StatusRejected, StatusFailed:
		return st, nil
	default:
		return "", ErrInvalidStatus
	}
}

// IsTerminal reports whether no further transitions are allowed.
func (s Status) IsTerminal() bool {
	return s == StatusProcessed || s == StatusRejected || s == StatusFailed
}
