package usecase

import "errors"

var (
	ErrNotFound     = errors.New("usecase: not found")
	ErrConflict     = errors.New("usecase: conflict")
	ErrInvalidInput = errors.New("usecase: invalid input")
)
