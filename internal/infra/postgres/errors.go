package postgres

import "errors"

var (
	ErrNotFound = errors.New("postgres: not found")
	ErrConflict = errors.New("postgres: conflict")
)
