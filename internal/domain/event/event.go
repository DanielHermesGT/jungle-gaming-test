package event

import (
	"encoding/json"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wager"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wallet"
)

const (
	TypeWagerProcessed        = "WagerTransactionProcessed"
	TypeWagerRejected         = "WagerTransactionRejected"
	TypeWalletBalanceChanged  = "WalletBalanceChanged"
	TypeWagerPendingReference = "WagerTransactionPendingReference"

	AggregateWager  = "WagerTransaction"
	AggregateWallet = "Wallet"
	Version         = 1
)

// Envelope is the immutable outbox/SQS event wrapper (README §11).
type Envelope struct {
	EventID       string          `json:"eventId"`
	EventType     string          `json:"eventType"`
	AggregateID   string          `json:"aggregateId"`
	CorrelationID string          `json:"correlationId"`
	CausationID   string          `json:"causationId,omitempty"`
	OccurredAt    time.Time       `json:"occurredAt"`
	Version       int             `json:"version"`
	Data          json.RawMessage `json:"data"`
}

type moneyData struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

func moneyDTO(m money.Money) moneyData {
	return moneyData{Amount: m.AmountString(), Currency: m.Currency()}
}

type WagerProcessedData struct {
	TransactionID         string    `json:"transactionId"`
	ProviderID            string    `json:"providerId,omitempty"`
	ExternalTransactionID string    `json:"externalTransactionId,omitempty"`
	WalletID              string    `json:"walletId"`
	PlayerID              string    `json:"playerId"`
	Kind                  string    `json:"kind"`
	Status                string    `json:"status"`
	Money                 moneyData `json:"money"`
}

type WagerRejectedData struct {
	TransactionID string    `json:"transactionId"`
	WalletID      string    `json:"walletId"`
	Kind          string    `json:"kind"`
	FailureCode   string    `json:"failureCode"`
	Money         moneyData `json:"money"`
}

type BalanceChangedData struct {
	WalletID      string    `json:"walletId"`
	TransactionID string    `json:"transactionId"`
	Direction     string    `json:"direction"`
	Money         moneyData `json:"money"`
	BalanceBefore moneyData `json:"balanceBefore"`
	BalanceAfter  moneyData `json:"balanceAfter"`
	WalletVersion int64     `json:"walletVersion"`
}

type PendingReferenceData struct {
	TransactionID                string     `json:"transactionId"`
	WalletID                     string     `json:"walletId"`
	Kind                         string     `json:"kind"`
	ReferenceExternalTransaction string     `json:"referenceExternalTransactionId"`
	PendingReferenceUntil        *time.Time `json:"pendingReferenceUntil,omitempty"`
}

func NewWagerProcessed(eventID, correlationID string, tx wager.Transaction, now time.Time) (Envelope, error) {
	data, err := json.Marshal(WagerProcessedData{
		TransactionID:         tx.ID(),
		ProviderID:            tx.ProviderID(),
		ExternalTransactionID: tx.ExternalTransactionID(),
		WalletID:              tx.WalletID(),
		PlayerID:              tx.PlayerID(),
		Kind:                  string(tx.Kind()),
		Status:                string(tx.Status()),
		Money:                 moneyDTO(tx.Amount()),
	})
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		EventID: eventID, EventType: TypeWagerProcessed, AggregateID: tx.ID(),
		CorrelationID: correlationID, OccurredAt: now.UTC(), Version: Version, Data: data,
	}, nil
}

func NewWagerRejected(eventID, correlationID string, tx wager.Transaction, now time.Time) (Envelope, error) {
	data, err := json.Marshal(WagerRejectedData{
		TransactionID: tx.ID(),
		WalletID:      tx.WalletID(),
		Kind:          string(tx.Kind()),
		FailureCode:   string(tx.FailureCode()),
		Money:         moneyDTO(tx.Amount()),
	})
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		EventID: eventID, EventType: TypeWagerRejected, AggregateID: tx.ID(),
		CorrelationID: correlationID, OccurredAt: now.UTC(), Version: Version, Data: data,
	}, nil
}

func NewWalletBalanceChanged(
	eventID, correlationID string,
	walletID, transactionID string,
	direction wallet.Direction,
	amount, before, after money.Money,
	walletVersion int64,
	now time.Time,
) (Envelope, error) {
	data, err := json.Marshal(BalanceChangedData{
		WalletID:      walletID,
		TransactionID: transactionID,
		Direction:     string(direction),
		Money:         moneyDTO(amount),
		BalanceBefore: moneyDTO(before),
		BalanceAfter:  moneyDTO(after),
		WalletVersion: walletVersion,
	})
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		EventID: eventID, EventType: TypeWalletBalanceChanged, AggregateID: walletID,
		CorrelationID: correlationID, OccurredAt: now.UTC(), Version: Version, Data: data,
	}, nil
}

func NewWagerPendingReference(eventID, correlationID string, tx wager.Transaction, now time.Time) (Envelope, error) {
	var until *time.Time
	if u := tx.PendingReferenceUntil(); !u.IsZero() {
		t := u.UTC()
		until = &t
	}
	data, err := json.Marshal(PendingReferenceData{
		TransactionID:                tx.ID(),
		WalletID:                     tx.WalletID(),
		Kind:                         string(tx.Kind()),
		ReferenceExternalTransaction: tx.ReferenceExternalTransaction(),
		PendingReferenceUntil:        until,
	})
	if err != nil {
		return Envelope{}, err
	}
	return Envelope{
		EventID: eventID, EventType: TypeWagerPendingReference, AggregateID: tx.ID(),
		CorrelationID: correlationID, OccurredAt: now.UTC(), Version: Version, Data: data,
	}, nil
}

func MarshalEnvelope(e Envelope) (json.RawMessage, error) {
	return json.Marshal(e)
}

func AggregateTypeFor(eventType string) string {
	if eventType == TypeWalletBalanceChanged {
		return AggregateWallet
	}
	return AggregateWager
}
