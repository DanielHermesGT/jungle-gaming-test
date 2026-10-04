package wager

import "go.uber.org/fx"

// Module provides the wager use case (reuses idgen/clock from wallet module).
var Module = fx.Module("usecase.wager",
	fx.Provide(NewUseCase),
)
