package messaging

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/usecase"
	usecasewager "github.com/DanielHermesGT/jungle-gaming-test/internal/usecase/wager"
)

// wagerProcessor is the SQS entry into the wager use case.
type wagerProcessor interface {
	ProcessFromQueue(ctx context.Context, consumerName, messageID, payloadHash string, in usecasewager.ProcessInput) (usecasewager.ProcessResult, error)
}

// Consumer long-polls wager-transactions.fifo and processes via ProcessFromQueue.
type Consumer struct {
	sqs *Client
	uc  wagerProcessor
}

func NewConsumer(sqsClient *Client, uc *usecasewager.UseCase) *Consumer {
	return &Consumer{sqs: sqsClient, uc: uc}
}

func (c *Consumer) Run(ctx context.Context) {
	slog.Info("messaging: sqs consumer started", "queue", c.sqs.wagerQueueURL)
	for {
		if ctx.Err() != nil {
			return
		}
		out, err := c.sqs.api.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(c.sqs.wagerQueueURL),
			MaxNumberOfMessages: 5,
			WaitTimeSeconds:     10,
			VisibilityTimeout:   30,
		})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("messaging: receive failed", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		for _, msg := range out.Messages {
			c.handle(ctx, msg)
		}
	}
}

func (c *Consumer) handle(ctx context.Context, msg types.Message) {
	body := aws.ToString(msg.Body)
	receipt := aws.ToString(msg.ReceiptHandle)

	parsed, err := parseInbound([]byte(body))
	if err != nil {
		slog.Warn("messaging: invalid message → DLQ", "err", err)
		c.sendDLQ(ctx, body, "invalid:"+err.Error())
		c.delete(ctx, receipt)
		return
	}

	_, err = c.uc.ProcessFromQueue(ctx, ConsumerWagerTransactions, parsed.MessageID, parsed.PayloadHash, parsed.Input)
	if err == nil {
		c.delete(ctx, receipt)
		return
	}

	switch {
	case errors.Is(err, usecase.ErrPermanent), errors.Is(err, usecase.ErrInvalidInput), errors.Is(err, usecase.ErrConflict):
		slog.Warn("messaging: permanent failure → DLQ", "messageId", parsed.MessageID, "err", err)
		c.sendDLQ(ctx, body, truncateErr(err))
		c.delete(ctx, receipt)
	default:
		// Transient: leave for visibility timeout retry. Do not log financial payload.
		slog.Error("messaging: transient failure", "messageID", parsed.MessageID, "err", err)
	}
}

func (c *Consumer) delete(ctx context.Context, receipt string) {
	if receipt == "" {
		return
	}
	_, err := c.sqs.api.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(c.sqs.wagerQueueURL),
		ReceiptHandle: aws.String(receipt),
	})
	if err != nil {
		slog.Error("messaging: delete failed", "err", err)
	}
}

func (c *Consumer) sendDLQ(ctx context.Context, body, reason string) {
	_, err := c.sqs.api.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:               aws.String(c.sqs.wagerDLQURL),
		MessageBody:            aws.String(body),
		MessageGroupId:         aws.String("dlq"),
		MessageDeduplicationId: aws.String(fmtHash(body + reason)),
	})
	if err != nil {
		slog.Error("messaging: dlq send failed", "err", err)
	}
}

func fmtHash(s string) string {
	// Dedup id max 128 chars; use sha256 hex.
	sum := sha256Hex([]byte(s))
	return sum
}
