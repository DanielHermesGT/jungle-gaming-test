package app

import (
	"go.uber.org/fx"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/config"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/database"
	usecasewallet "github.com/DanielHermesGT/jungle-gaming-test/internal/usecase/wallet"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/web"
)

// Module composes config, database, wallet use cases and HTTP.
var Module = fx.Options(
	config.Module,
	database.Module,
	usecasewallet.Module,
	web.Module,
)
