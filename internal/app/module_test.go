package app_test

import (
	"testing"

	"go.uber.org/fx"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/app"
)

func TestFxAppValidates(t *testing.T) {
	if err := fx.ValidateApp(app.Module); err != nil {
		t.Fatalf("fx validate: %v", err)
	}
}
