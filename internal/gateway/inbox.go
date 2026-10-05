package gateway

import (
	"context"
	"time"
)

// InboxMessage tracks durable processing of an inbound transport message.
type InboxMessage struct {
	ConsumerName string
	MessageID    string
	PayloadHash  string
	ReceivedAt   time.Time
	CompletedAt  *time.Time
}

// InboxRepository persists SQS (or other) inbox deduplication state.
type InboxRepository interface {
	Get(ctx context.Context, q Querier, consumerName, messageID string) (InboxMessage, error)
	InsertReceived(ctx context.Context, q Querier, msg InboxMessage) error
	MarkCompleted(ctx context.Context, q Querier, consumerName, messageID string, at time.Time) error
}
