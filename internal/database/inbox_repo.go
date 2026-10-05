package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/gateway"
)

type InboxRepo struct{}

func NewInboxRepo() *InboxRepo { return &InboxRepo{} }

var _ gateway.InboxRepository = (*InboxRepo)(nil)

func (r *InboxRepo) Get(ctx context.Context, q gateway.Querier, consumerName, messageID string) (gateway.InboxMessage, error) {
	const sql = `
SELECT consumer_name, message_id, payload_hash, received_at, completed_at
FROM inbox_messages
WHERE consumer_name = $1 AND message_id = $2`

	var msg gateway.InboxMessage
	var completed *time.Time
	err := q.QueryRow(ctx, sql, consumerName, messageID).Scan(
		&msg.ConsumerName, &msg.MessageID, &msg.PayloadHash, &msg.ReceivedAt, &completed,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return gateway.InboxMessage{}, ErrNotFound
	}
	if err != nil {
		return gateway.InboxMessage{}, fmt.Errorf("database: get inbox: %w", err)
	}
	msg.CompletedAt = completed
	return msg, nil
}

func (r *InboxRepo) InsertReceived(ctx context.Context, q gateway.Querier, msg gateway.InboxMessage) error {
	const sql = `
INSERT INTO inbox_messages (consumer_name, message_id, payload_hash, received_at)
VALUES ($1, $2, $3, $4)`
	_, err := q.Exec(ctx, sql, msg.ConsumerName, msg.MessageID, msg.PayloadHash, msg.ReceivedAt.UTC())
	if isUniqueViolation(err) {
		return fmt.Errorf("%w: inbox message", ErrConflict)
	}
	if err != nil {
		return fmt.Errorf("database: insert inbox: %w", err)
	}
	return nil
}

func (r *InboxRepo) MarkCompleted(
	ctx context.Context,
	q gateway.Querier,
	consumerName, messageID string,
	at time.Time,
) error {
	const sql = `
UPDATE inbox_messages
SET completed_at = $3
WHERE consumer_name = $1 AND message_id = $2`
	tag, err := q.Exec(ctx, sql, consumerName, messageID, at.UTC())
	if err != nil {
		return fmt.Errorf("database: complete inbox: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
