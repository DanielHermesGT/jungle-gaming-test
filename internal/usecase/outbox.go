package usecase

import (
	"encoding/json"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/event"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/gateway"
)

// OutboxRecordFromEnvelope maps a domain event envelope to a persistence record.
func OutboxRecordFromEnvelope(e event.Envelope, now time.Time) (gateway.OutboxRecord, error) {
	payload, err := event.MarshalEnvelope(e)
	if err != nil {
		return gateway.OutboxRecord{}, err
	}
	headers, err := json.Marshal(map[string]any{
		"correlationId": e.CorrelationID,
		"causationId":   e.CausationID,
		"version":       e.Version,
	})
	if err != nil {
		return gateway.OutboxRecord{}, err
	}
	return gateway.OutboxRecord{
		ID:            e.EventID,
		EventType:     e.EventType,
		AggregateID:   e.AggregateID,
		AggregateType: event.AggregateTypeFor(e.EventType),
		Payload:       payload,
		Headers:       headers,
		OccurredAt:    e.OccurredAt,
		CreatedAt:     now.UTC(),
		NextAttemptAt: now.UTC(),
	}, nil
}
