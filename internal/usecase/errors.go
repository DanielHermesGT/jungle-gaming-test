package usecase

import "errors"

var (
	ErrNotFound     = errors.New("usecase: not found")
	ErrConflict     = errors.New("usecase: conflict")
	ErrInvalidInput = errors.New("usecase: invalid input")
	// ErrPermanent is a non-retryable processing failure (e.g. inbox payload hash mismatch → DLQ).
	ErrPermanent = errors.New("usecase: permanent failure")
)
