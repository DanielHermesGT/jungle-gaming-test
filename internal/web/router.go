package web

import (
	"net/http"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/auth"
)

// NewRouter mounts HTTP routes.
// Health is public. /wallets* require JWT + internal role (README §2).
func NewRouter(wallets *WalletHandler, health *HealthHandler, mw *auth.Middleware) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health/live", health.Live)
	mux.HandleFunc("GET /health/ready", health.Ready)

	mux.Handle("POST /wallets", mw.ProtectInternal(http.HandlerFunc(wallets.Open)))
	mux.Handle("GET /wallets/{walletId}", mw.ProtectInternal(http.HandlerFunc(wallets.Get)))
	mux.Handle("GET /wallets/{walletId}/ledger", mw.ProtectInternal(http.HandlerFunc(wallets.ListLedger)))
	mux.Handle("POST /wallets/{walletId}/reconciliation", mw.ProtectInternal(http.HandlerFunc(wallets.Reconcile)))

	return mux
}
