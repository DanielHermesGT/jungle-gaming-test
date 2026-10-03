package web

import "net/http"

// NewRouter mounts HTTP routes.
//
// TODO(futuro): proteger /wallets* com Keycloak JWT (JWKS) + role de serviço interno
// (README §2 — eliminatório). Health permanece público.
func NewRouter(wallets *WalletHandler, health *HealthHandler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health/live", health.Live)
	mux.HandleFunc("GET /health/ready", health.Ready)

	mux.HandleFunc("POST /wallets", wallets.Open)
	mux.HandleFunc("GET /wallets/{walletId}", wallets.Get)
	mux.HandleFunc("GET /wallets/{walletId}/ledger", wallets.ListLedger)
	mux.HandleFunc("POST /wallets/{walletId}/reconciliation", wallets.Reconcile)

	return mux
}
