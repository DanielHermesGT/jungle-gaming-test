package gateway

import "errors"

var (
	ErrNotFound = errors.New("gateway: not found")
	ErrConflict = errors.New("gateway: conflict")
)
