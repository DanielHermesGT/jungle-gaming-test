package config

import (
	"fmt"
	"os"
)

// Config holds process configuration from the environment.
type Config struct {
	HTTPAddr      string
	DatabaseURL   string
	OIDCIssuerURL string
	OIDCAudience  string
	InternalRole  string

	AWSRegion          string
	AWSEndpointURL     string
	AWSAccessKeyID     string
	AWSSecretAccessKey string

	SQSWagerQueueURL        string
	SQSWagerDLQURL          string
	SQSDomainEventsQueueURL string
}

// Load reads configuration from environment variables.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:      envOr("HTTP_ADDR", ":8080"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		OIDCIssuerURL: os.Getenv("OIDC_ISSUER_URL"),
		OIDCAudience:  envOr("OIDC_AUDIENCE", "jungle-api"),
		InternalRole:  envOr("INTERNAL_SERVICE_ROLE", "wallet-internal"),

		AWSRegion:          envOr("AWS_REGION", "us-east-1"),
		AWSEndpointURL:     envOr("AWS_ENDPOINT_URL", "http://localhost:4566"),
		AWSAccessKeyID:     envOr("AWS_ACCESS_KEY_ID", "test"),
		AWSSecretAccessKey: envOr("AWS_SECRET_ACCESS_KEY", "test"),

		SQSWagerQueueURL:        os.Getenv("SQS_WAGER_QUEUE_URL"),
		SQSWagerDLQURL:          os.Getenv("SQS_WAGER_DLQ_URL"),
		SQSDomainEventsQueueURL: os.Getenv("SQS_DOMAIN_EVENTS_QUEUE_URL"),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("config: DATABASE_URL is required")
	}
	if cfg.OIDCIssuerURL == "" {
		return Config{}, fmt.Errorf("config: OIDC_ISSUER_URL is required")
	}
	if cfg.SQSWagerQueueURL == "" {
		return Config{}, fmt.Errorf("config: SQS_WAGER_QUEUE_URL is required")
	}
	if cfg.SQSWagerDLQURL == "" {
		return Config{}, fmt.Errorf("config: SQS_WAGER_DLQ_URL is required")
	}
	if cfg.SQSDomainEventsQueueURL == "" {
		return Config{}, fmt.Errorf("config: SQS_DOMAIN_EVENTS_QUEUE_URL is required")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
