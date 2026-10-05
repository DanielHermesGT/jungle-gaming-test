package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/event"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/gateway"
	"github.com/DanielHermesGT/jungle-gaming-test/pkg/clock"
	"github.com/DanielHermesGT/jungle-gaming-test/pkg/idgen"
)

const (
	publisherBatchSize = 10
	publisherIdleWait  = 2 * time.Second
)

// Publisher claims outbox rows and publishes them to domain-events.fifo.
type Publisher struct {
	db      gateway.DB
	outbox  gateway.OutboxRepository
	sqs     *Client
	ids     idgen.Generator
	clock   clock.Clock
	workerID string
}

func NewPublisher(
	db gateway.DB,
	outbox gateway.OutboxRepository,
	sqsClient *Client,
	ids idgen.Generator,
	clk clock.Clock,
) *Publisher {
	return &Publisher{
		db:       db,
		outbox:   outbox,
		sqs:      sqsClient,
		ids:      ids,
		clock:    clk,
		workerID: "publisher-" + ids.New(),
	}
}

func (p *Publisher) Run(ctx context.Context) {
	slog.Info("messaging: outbox publisher started", "workerId", p.workerID)
	for {
		if ctx.Err() != nil {
			return
		}
		n, err := p.publishOnce(ctx)
		if err != nil {
			slog.Error("messaging: publisher tick failed", "err", err)
		}
		if n == 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(publisherIdleWait):
			}
		}
	}
}

func (p *Publisher) publishOnce(ctx context.Context) (int, error) {
	now := p.clock.Now().UTC()
	var claimed []gateway.OutboxRecord
	err := p.db.WithinTx(ctx, func(q gateway.Querier) error {
		var err error
		claimed, err = p.outbox.ClaimBatch(ctx, q, p.workerID, publisherBatchSize, now)
		return err
	})
	if err != nil {
		return 0, err
	}
	for _, rec := range claimed {
		if err := p.publishOne(ctx, rec); err != nil {
			slog.Warn("messaging: publish failed", "eventId", rec.ID, "eventType", rec.EventType)
			_ = p.db.WithinTx(ctx, func(q gateway.Querier) error {
				return p.outbox.MarkRetry(ctx, q, rec.ID, rec.Attempts, backoffNext(rec.Attempts, now), truncateErr(err))
			})
			continue
		}
		_ = p.db.WithinTx(ctx, func(q gateway.Querier) error {
			return p.outbox.MarkPublished(ctx, q, rec.ID, p.clock.Now().UTC())
		})
	}
	return len(claimed), nil
}

func (p *Publisher) publishOne(ctx context.Context, rec gateway.OutboxRecord) error {
	var env event.Envelope
	if err := json.Unmarshal(rec.Payload, &env); err != nil {
		return fmt.Errorf("unmarshal envelope: %w", err)
	}
	groupID := env.AggregateID
	if groupID == "" {
		groupID = rec.AggregateID
	}
	_, err := p.sqs.api.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:               aws.String(p.sqs.eventsQueueURL),
		MessageBody:            aws.String(string(rec.Payload)),
		MessageGroupId:         aws.String(groupID),
		MessageDeduplicationId: aws.String(rec.ID), // preserve eventId across republish
	})
	return err
}

func backoffNext(attempts int, now time.Time) time.Time {
	// 1s, 2s, 4s, ... capped at 5m
	sec := 1 << min(attempts, 8)
	if sec > 300 {
		sec = 300
	}
	return now.Add(time.Duration(sec) * time.Second)
}

func truncateErr(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > 500 {
		return s[:500]
	}
	return s
}
