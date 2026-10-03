package auth

import "context"

const (
	// RoleWalletInternal authorizes wallet HTTP operations (README §2).
	RoleWalletInternal = "wallet-internal"
)

type ctxKey int

const principalKey ctxKey = 1

// Principal is the authenticated caller identity.
type Principal struct {
	Subject    string
	Roles      []string
	ProviderID string // set for provider clients; empty for internal service
}

func (p Principal) HasRole(role string) bool {
	for _, r := range p.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// PrincipalFromContext returns the authenticated principal, if any.
// recupera o valor armazenado no contexto
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey).(Principal)
	return p, ok
}
