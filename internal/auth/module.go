package auth

import (
	"context"

	"go.uber.org/fx"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/config"
)

// Module provides OIDC token verification and HTTP auth middleware.
var Module = fx.Module("auth",
	fx.Provide(
		NewOIDCVerifierFromConfig,
		NewMiddlewareFromConfig,
	),
)

func NewOIDCVerifierFromConfig(cfg config.Config) (TokenVerifier, error) {
	return NewOIDCVerifier(context.Background(), cfg)
}

func NewMiddlewareFromConfig(v TokenVerifier, cfg config.Config) *Middleware {
	return NewMiddleware(v, cfg.InternalRole)
}
