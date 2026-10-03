package database

import (
	"context"

	"go.uber.org/fx"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/config"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/gateway"
)

// Module provides PostgreSQL connectivity and repositories.
var Module = fx.Module("database",
	fx.Provide(
		NewDBFromConfig,
		func(db *DB) gateway.DB { return db },
		fx.Annotate(NewWalletRepo, fx.As(new(gateway.WalletRepository))),
		fx.Annotate(NewLedgerRepo, fx.As(new(gateway.LedgerRepository))),
	),
	fx.Invoke(registerDBLifecycle),
)

func NewDBFromConfig(cfg config.Config) (*DB, error) {
	return NewDB(context.Background(), cfg.DatabaseURL)
}

func registerDBLifecycle(lc fx.Lifecycle, db *DB) {
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error {
			db.Close()
			return nil
		},
	})
}
