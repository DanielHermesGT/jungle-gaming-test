package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/gateway"
)

type OutboxRepo struct{}

func NewOutboxRepo() *OutboxRepo { return &OutboxRepo{} }

var _ gateway.OutboxRepository = (*OutboxRepo)(nil)

func (r *OutboxRepo) Insert(ctx context.Context, q gateway.Querier, records ...gateway.OutboxRecord) error {
	const sql = `
INSERT INTO outbox_events (
    id, event_type, aggregate_id, aggregate_type, payload, headers,
    occurred_at, created_at, attempts, next_attempt_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`

	for _, rec := range records {
		headers := rec.Headers
		if len(headers) == 0 {
			headers = json.RawMessage(`{}`)
		}
		_, err := q.Exec(ctx, sql,
			rec.ID,
			rec.EventType,
			rec.AggregateID,
			rec.AggregateType,
			[]byte(rec.Payload),
			[]byte(headers),
			rec.OccurredAt,
			rec.CreatedAt,
			rec.Attempts,
			rec.NextAttemptAt,
		)
		if err != nil {
			return fmt.Errorf("database: insert outbox: %w", err)
		}
	}
	return nil
}

func (r *OutboxRepo) ClaimBatch(
	ctx context.Context,
	q gateway.Querier,
	workerID string,
	limit int,
	now time.Time,
) ([]gateway.OutboxRecord, error) {
	if limit < 1 {
		limit = 10
	}
	const sql = `
WITH cte AS (
    SELECT id
    FROM outbox_events
    WHERE published_at IS NULL
      AND next_attempt_at <= $1
      AND (locked_at IS NULL OR locked_at < $1 - INTERVAL '30 seconds')
    ORDER BY next_attempt_at ASC, id ASC
    FOR UPDATE SKIP LOCKED
    LIMIT $2
)
UPDATE outbox_events o
SET locked_at = $1,
    locked_by = $3,
    attempts = o.attempts + 1
FROM cte
WHERE o.id = cte.id
RETURNING o.id, o.event_type, o.aggregate_id, o.aggregate_type, o.payload, o.headers,
          o.occurred_at, o.created_at, o.published_at, o.attempts, o.next_attempt_at,
          o.last_error, o.locked_at, o.locked_by`

	rows, err := q.Query(ctx, sql, now.UTC(), limit, workerID)
	if err != nil {
		return nil, fmt.Errorf("database: claim outbox: %w", err)
	}
	defer rows.Close()

	out := make([]gateway.OutboxRecord, 0, limit)
	for rows.Next() {
		var rec gateway.OutboxRecord
		var payload, headers []byte
		var publishedAt, lockedAt *time.Time
		var lastErr, lockedBy *string
		if err := rows.Scan(
			&rec.ID, &rec.EventType, &rec.AggregateID, &rec.AggregateType,
			&payload, &headers, &rec.OccurredAt, &rec.CreatedAt, &publishedAt,
			&rec.Attempts, &rec.NextAttemptAt, &lastErr, &lockedAt, &lockedBy,
		); err != nil {
			return nil, fmt.Errorf("database: scan outbox: %w", err)
		}
		rec.Payload = json.RawMessage(payload)
		rec.Headers = json.RawMessage(headers)
		rec.PublishedAt = publishedAt
		rec.LockedAt = lockedAt
		if lastErr != nil {
			rec.LastError = *lastErr
		}
		if lockedBy != nil {
			rec.LockedBy = *lockedBy
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (r *OutboxRepo) MarkPublished(ctx context.Context, q gateway.Querier, eventID string, publishedAt time.Time) error {
	const sql = `
UPDATE outbox_events
SET published_at = $2, locked_at = NULL, locked_by = NULL, last_error = NULL
WHERE id = $1`
	tag, err := q.Exec(ctx, sql, eventID, publishedAt.UTC())
	if err != nil {
		return fmt.Errorf("database: mark published: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *OutboxRepo) MarkRetry(
	ctx context.Context,
	q gateway.Querier,
	eventID string,
	attempts int,
	next time.Time,
	lastErr string,
) error {
	if len(lastErr) > 500 {
		lastErr = lastErr[:500]
	}
	const sql = `
UPDATE outbox_events
SET attempts = $2, next_attempt_at = $3, last_error = $4, locked_at = NULL, locked_by = NULL
WHERE id = $1`
	tag, err := q.Exec(ctx, sql, eventID, attempts, next.UTC(), lastErr)
	if err != nil {
		return fmt.Errorf("database: mark retry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
