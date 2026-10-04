package auth

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
)

// Middleware authenticates JWTs and enforces authorization rules.
type Middleware struct {
	verifier     TokenVerifier
	internalRole string
}

func NewMiddleware(verifier TokenVerifier, internalRole string) *Middleware {
	if internalRole == "" {
		internalRole = RoleWalletInternal
	}
	return &Middleware{verifier: verifier, internalRole: internalRole}
}

// ProtectInternal = Authenticate then RequireInternal (wallet routes).
func (m *Middleware) ProtectInternal(next http.Handler) http.Handler {
	return m.Authenticate(m.RequireInternal(next))
}

// Authenticate validates Bearer JWT and stores Principal in context.
func (m *Middleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			writeAuthError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid authorization")
			return
		}
		principal, err := m.verifier.Verify(r.Context(), raw)
		if err != nil {
			slog.Warn("auth: token rejected")
			writeAuthError(w, http.StatusUnauthorized, "unauthorized", "invalid token")
			return
		}
		ctx := context.WithValue(r.Context(), principalKey, principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireInternal allows only principals with the internal wallet role.
func (m *Middleware) RequireInternal(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFromContext(r.Context())
		if !ok || !p.HasRole(m.internalRole) {
			writeAuthError(w, http.StatusForbidden, "forbidden", "internal service role required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireProvider ensures the principal's providerId matches the expected value.
func (m *Middleware) RequireProvider(providerID string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := PrincipalFromContext(r.Context())
		if !ok || p.ProviderID == "" || p.ProviderID != providerID {
			writeAuthError(w, http.StatusForbidden, "forbidden", "provider not authorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ProtectProviderPath autentica e exige que PathValue(pathKey) == Principal.ProviderID.
// Uso: GET /providers/{providerId}/wagering/...
func (m *Middleware) ProtectProviderPath(pathKey string, next http.Handler) http.Handler {
	return m.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerID := r.PathValue(pathKey)
		m.RequireProvider(providerID, next).ServeHTTP(w, r)
	}))
}

func bearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	tok := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return tok, tok != ""
}

type authErrorBody struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func writeAuthError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(authErrorBody{Error: message, Code: code})
}
