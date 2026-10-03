package web

import "github.com/DanielHermesGT/jungle-gaming-test/internal/database"

// NewWalletHandlerForTest builds a handler with a test double.
func NewWalletHandlerForTest(uc walletService) *WalletHandler {
	return &WalletHandler{uc: uc}
}

// NewHealthHandlerForTest returns a health handler without a live DB.
// Ready is not exercised in unit tests that use this helper.
func NewHealthHandlerForTest() *HealthHandler {
	return &HealthHandler{db: &database.DB{}}
}
