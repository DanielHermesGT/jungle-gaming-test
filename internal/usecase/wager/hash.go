package wager

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
	domainwager "github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wager"
)

// PayloadFields are business fields included in the idempotency payload hash.
// Excludes Idempotency-Key and transport metadata (README §8).
type PayloadFields struct {
	ProviderID                     string
	ExternalTransactionID          string
	PlayerID                       string
	WalletID                       string
	RoundID                        string
	GameID                         string
	Kind                           domainwager.Kind
	Amount                         money.Money
	ReferenceExternalTransactionID string
}

// CanonicalPayloadHash returns SHA-256 hex of canonical JSON with sorted keys.
func CanonicalPayloadHash(f PayloadFields) (string, error) {
	if f.Amount.Currency() == "" {
		return "", fmt.Errorf("payload hash: uninitialized amount")
	}
	m := map[string]string{
		"amount":                f.Amount.AmountString(),
		"currency":              f.Amount.Currency(),
		"externalTransactionId": f.ExternalTransactionID,
		"gameId":                f.GameID,
		"kind":                  string(f.Kind),
		"playerId":              f.PlayerID,
		"providerId":            f.ProviderID,
		"roundId":               f.RoundID,
		"walletId":              f.WalletID,
	}
	if f.ReferenceExternalTransactionID != "" {
		m["referenceExternalTransactionId"] = f.ReferenceExternalTransactionID
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := make(map[string]string, len(m))
	for _, k := range keys {
		ordered[k] = m[k]
	}
	raw, err := json.Marshal(ordered)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
