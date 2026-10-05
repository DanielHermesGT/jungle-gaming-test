package messaging

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
	domainwager "github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wager"
	usecasewager "github.com/DanielHermesGT/jungle-gaming-test/internal/usecase/wager"
)

const ConsumerWagerTransactions = "wager-transactions"

type inboundEnvelope struct {
	MessageID  string          `json:"messageId"`
	Type       string          `json:"type"`
	OccurredAt string          `json:"occurredAt"`
	Data       json.RawMessage `json:"data"`
}

type inboundData struct {
	ProviderID                     string `json:"providerId"`
	ExternalTransactionID          string `json:"externalTransactionId"`
	IdempotencyKey                 string `json:"idempotencyKey"`
	PlayerID                       string `json:"playerId"`
	WalletID                       string `json:"walletId"`
	RoundID                        string `json:"roundId"`
	GameID                         string `json:"gameId"`
	Kind                           string `json:"kind"`
	ReferenceExternalTransactionID string `json:"referenceExternalTransactionId"`
	Money                          struct {
		Amount   string `json:"amount"`
		Currency string `json:"currency"`
	} `json:"money"`
}

type parsedInbound struct {
	MessageID   string
	PayloadHash string
	Input       usecasewager.ProcessInput
}

func parseInbound(body []byte) (parsedInbound, error) {
	var env inboundEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return parsedInbound{}, fmt.Errorf("invalid json: %w", err)
	}
	if env.MessageID == "" {
		return parsedInbound{}, fmt.Errorf("missing messageId")
	}
	var data inboundData
	if err := json.Unmarshal(env.Data, &data); err != nil {
		return parsedInbound{}, fmt.Errorf("invalid data: %w", err)
	}

	amt, err := money.Parse(data.Money.Currency, data.Money.Amount)
	if err != nil {
		return parsedInbound{}, fmt.Errorf("money: %w", err)
	}
	kind := domainwager.Kind(data.Kind)
	hash, err := usecasewager.CanonicalPayloadHash(usecasewager.PayloadFields{
		ProviderID:                     data.ProviderID,
		ExternalTransactionID:          data.ExternalTransactionID,
		PlayerID:                       data.PlayerID,
		WalletID:                       data.WalletID,
		RoundID:                        data.RoundID,
		GameID:                         data.GameID,
		Kind:                           kind,
		Amount:                         amt,
		ReferenceExternalTransactionID: data.ReferenceExternalTransactionID,
	})
	if err != nil {
		return parsedInbound{}, err
	}

	return parsedInbound{
		MessageID:   env.MessageID,
		PayloadHash: sha256Hex(body),
		Input: usecasewager.ProcessInput{
			ProviderID:                     data.ProviderID,
			ExternalTransactionID:          data.ExternalTransactionID,
			IdempotencyKey:                 data.IdempotencyKey,
			PlayerID:                       data.PlayerID,
			WalletID:                       data.WalletID,
			RoundID:                        data.RoundID,
			GameID:                         data.GameID,
			Kind:                           kind,
			Amount:                         amt,
			ReferenceExternalTransactionID: data.ReferenceExternalTransactionID,
			PayloadHash:                    hash,
		},
	}, nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
