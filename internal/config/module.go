package config

import "go.uber.org/fx"

// Module provides application configuration.
var Module = fx.Module("config",
	fx.Provide(Load),
)
