package gateway

import (
	"context"
	"encoding/json"
	"time"
)

// OutboxRecord is a durable domain event waiting to be published.
type OutboxRecord struct {
	ID            string
	EventType     string
	AggregateID   string
	AggregateType string
	Payload       json.RawMessage
	Headers       json.RawMessage
	OccurredAt    time.Time
	CreatedAt     time.Time
	PublishedAt   *time.Time
	Attempts      int
	NextAttemptAt time.Time
	LastError     string
	LockedAt      *time.Time
	LockedBy      string
}

// OutboxRepository persists and claims outbox events.
type OutboxRepository interface {
	Insert(ctx context.Context, q Querier, records ...OutboxRecord) error
	ClaimBatch(ctx context.Context, q Querier, workerID string, limit int, now time.Time) ([]OutboxRecord, error)
	MarkPublished(ctx context.Context, q Querier, eventID string, publishedAt time.Time) error
	MarkRetry(ctx context.Context, q Querier, eventID string, attempts int, next time.Time, lastErr string) error
}
