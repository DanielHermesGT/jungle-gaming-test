package database

import "github.com/DanielHermesGT/jungle-gaming-test/internal/gateway"

// Re-export gateway sentinels so existing callers/tests keep working.
var (
	ErrNotFound = gateway.ErrNotFound
	ErrConflict = gateway.ErrConflict
)
