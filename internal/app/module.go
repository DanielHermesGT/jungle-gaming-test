package app

import (
	"go.uber.org/fx"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/auth"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/config"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/database"
	usecasewager "github.com/DanielHermesGT/jungle-gaming-test/internal/usecase/wager"
	usecasewallet "github.com/DanielHermesGT/jungle-gaming-test/internal/usecase/wallet"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/web"
)

// Module composes config, auth, database, use cases and HTTP.
var Module = fx.Options(
	config.Module,
	auth.Module,
	database.Module,
	usecasewallet.Module,
	usecasewager.Module,
	web.Module,
)
