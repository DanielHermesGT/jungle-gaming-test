package wallet

import (
	"go.uber.org/fx"

	"github.com/DanielHermesGT/jungle-gaming-test/pkg/clock"
	"github.com/DanielHermesGT/jungle-gaming-test/pkg/idgen"
)

// Module provides the wallet use case and defaults for IDs/time.
var Module = fx.Module("usecase.wallet",
	fx.Provide(
		func() idgen.Generator { return idgen.UUID{} },
		func() clock.Clock { return clock.System{} },
		NewUseCase,
	),
)
