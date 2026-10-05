package app

import (
	"go.uber.org/fx"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/auth"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/config"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/database"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/messaging"
	usecasewager "github.com/DanielHermesGT/jungle-gaming-test/internal/usecase/wager"
	usecasewallet "github.com/DanielHermesGT/jungle-gaming-test/internal/usecase/wallet"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/web"
)

// Module composes config, auth, database, use cases, messaging workers and HTTP.
var Module = fx.Options(
	config.Module,
	auth.Module,
	database.Module,
	usecasewallet.Module,
	usecasewager.Module,
	messaging.Module,
	fx.Provide(func(c *messaging.Client) web.QueueReadyChecker { return c }),
	web.Module,
)
