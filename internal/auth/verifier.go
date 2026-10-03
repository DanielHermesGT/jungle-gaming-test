package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/config"
)

// TokenVerifier validates a raw bearer token and returns a Principal.
type TokenVerifier interface {
	Verify(ctx context.Context, rawToken string) (Principal, error)
}

type oidcVerifier struct {
	verifier *oidc.IDTokenVerifier
}

// NewOIDCVerifier discovers the issuer JWKS and builds a token verifier.
func NewOIDCVerifier(ctx context.Context, cfg config.Config) (TokenVerifier, error) {
	provider, err := oidc.NewProvider(ctx, cfg.OIDCIssuerURL)
	if err != nil {
		return nil, fmt.Errorf("auth: oidc provider: %w", err)
	}
	v := provider.Verifier(&oidc.Config{
		ClientID: cfg.OIDCAudience,
	})
	return &oidcVerifier{verifier: v}, nil
}

func (v *oidcVerifier) Verify(ctx context.Context, rawToken string) (Principal, error) {
	tok, err := v.verifier.Verify(ctx, rawToken)
	if err != nil {
		return Principal{}, fmt.Errorf("auth: verify: %w", err)
	}

	var claims tokenClaims
	if err := tok.Claims(&claims); err != nil {
		return Principal{}, fmt.Errorf("auth: claims: %w", err)
	}

	roles := collectRoles(claims)
	providerID := claims.ProviderID
	if providerID == "" {
		providerID = claims.ProviderIDAlt
	}

	subject := claims.Subject
	if subject == "" {
		subject = tok.Subject
	}

	return Principal{
		Subject:    subject,
		Roles:      roles,
		ProviderID: providerID,
	}, nil
}

type tokenClaims struct {
	Subject       string `json:"sub"`
	ProviderID    string `json:"provider_id"`
	ProviderIDAlt string `json:"providerId"`
	RealmAccess   struct {
		Roles []string `json:"roles"`
	} `json:"realm_access"`
	ResourceAccess map[string]struct {
		Roles []string `json:"roles"`
	} `json:"resource_access"`
	Roles []string `json:"roles"`
}

func collectRoles(c tokenClaims) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(roles []string) {
		for _, r := range roles {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			if _, ok := seen[r]; ok {
				continue
			}
			seen[r] = struct{}{}
			out = append(out, r)
		}
	}
	add(c.Roles)
	add(c.RealmAccess.Roles)
	for _, ra := range c.ResourceAccess {
		add(ra.Roles)
	}
	return out
}
