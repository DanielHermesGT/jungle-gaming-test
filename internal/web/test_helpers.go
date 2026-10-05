package web

import (
	"context"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/auth"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/database"
)

// NewWalletHandlerForTest builds a handler with a test double.
func NewWalletHandlerForTest(uc walletService) *WalletHandler {
	return &WalletHandler{uc: uc}
}

// NewWagerHandlerForTest builds a wager handler with a test double.
func NewWagerHandlerForTest(uc wagerService) *WagerHandler {
	return &WagerHandler{uc: uc}
}

// NewHealthHandlerForTest returns a health handler without a live DB/SQS.
// Ready is not exercised in unit tests that use this helper.
func NewHealthHandlerForTest() *HealthHandler {
	return &HealthHandler{db: &database.DB{}, sqs: nil}
}

type stubTokenVerifier struct {
	principal auth.Principal
	err       error
}

func (s stubTokenVerifier) Verify(context.Context, string) (auth.Principal, error) {
	if s.err != nil {
		return auth.Principal{}, s.err
	}
	return s.principal, nil
}

// NewInternalAuthForTest accepts any Bearer token as an internal service principal.
func NewInternalAuthForTest() *auth.Middleware {
	return auth.NewMiddleware(stubTokenVerifier{
		principal: auth.Principal{
			Subject: "test-internal",
			Roles:   []string{auth.RoleWalletInternal},
		},
	}, auth.RoleWalletInternal)
}

// NewProviderAuthForTest accepts any Bearer token as a provider without internal role.
func NewProviderAuthForTest() *auth.Middleware {
	return auth.NewMiddleware(stubTokenVerifier{
		principal: auth.Principal{
			Subject:    "provider-a",
			ProviderID: "provider-a",
			Roles:      nil,
		},
	}, auth.RoleWalletInternal)
}
