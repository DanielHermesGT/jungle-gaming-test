package main

import (
	"go.uber.org/fx"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/app"
)

func main() {
	fx.New(app.Module).Run()
}
