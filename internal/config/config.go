package config

import (
	"fmt"
	"os"
)

// Config holds process configuration from the environment.
type Config struct {
	HTTPAddr      string
	DatabaseURL   string
	OIDCIssuerURL string //quem emitiu o token
	OIDCAudience  string //público do token
	InternalRole  string //role interno para acesso ao serviço
}

// Load reads configuration from environment variables.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:      envOr("HTTP_ADDR", ":8080"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		OIDCIssuerURL: os.Getenv("OIDC_ISSUER_URL"),
		OIDCAudience:  envOr("OIDC_AUDIENCE", "jungle-api"),
		InternalRole:  envOr("INTERNAL_SERVICE_ROLE", "wallet-internal"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("config: DATABASE_URL is required")
	}
	if cfg.OIDCIssuerURL == "" {
		return Config{}, fmt.Errorf("config: OIDC_ISSUER_URL is required")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
