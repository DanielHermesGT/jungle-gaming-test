package web

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"go.uber.org/fx"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/config"
)

// Module provides HTTP handlers and the server lifecycle.
var Module = fx.Module("web",
	fx.Provide(
		NewWalletHandler,
		NewWagerHandler,
		NewHealthHandler,
		NewRouter,
		NewHTTPServer,
	),
	fx.Invoke(registerHTTPServer),
)

func NewHTTPServer(cfg config.Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func registerHTTPServer(lc fx.Lifecycle, server *http.Server) {
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					slog.Error("web: listen failed", "err", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return server.Shutdown(ctx)
		},
	})
}
