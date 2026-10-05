package web

import (
	"net/http"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/auth"
)

// NewRouter mounts HTTP routes.
// Health is public. /wallets* require internal role. /wagering* require provider JWT.
func NewRouter(wallets *WalletHandler, wagers *WagerHandler, health *HealthHandler, mw *auth.Middleware) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health/live", health.Live)
	mux.HandleFunc("GET /health/ready", health.Ready)
	mux.HandleFunc("GET /metrics", health.Metrics)

	mux.Handle("POST /wallets", mw.ProtectInternal(http.HandlerFunc(wallets.Open)))
	mux.Handle("GET /wallets/{walletId}", mw.ProtectInternal(http.HandlerFunc(wallets.Get)))
	mux.Handle("GET /wallets/{walletId}/ledger", mw.ProtectInternal(http.HandlerFunc(wallets.ListLedger)))
	mux.Handle("POST /wallets/{walletId}/reconciliation", mw.ProtectInternal(http.HandlerFunc(wallets.Reconcile)))

	mux.Handle("POST /wagering/transactions", mw.Authenticate(http.HandlerFunc(wagers.Process)))
	mux.Handle("GET /wagering/transactions/{transactionId}", mw.Authenticate(http.HandlerFunc(wagers.Get)))
	mux.Handle(
		"GET /providers/{providerId}/wagering/transactions/{externalTransactionId}",
		mw.ProtectProviderPath("providerId", http.HandlerFunc(wagers.GetByExternal)),
	)

	return withRequestLog(mux)
}
