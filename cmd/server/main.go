package main

import (
	"go.uber.org/fx"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/app"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/observability"
)

func main() {
	observability.SetupJSON()
	fx.New(app.Module).Run()
}
