package messaging

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/observability"
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
	wg  sync.WaitGroup
}

func NewConsumer(sqsClient *Client, uc *usecasewager.UseCase) *Consumer {
	return &Consumer{sqs: sqsClient, uc: uc}
}

func (c *Consumer) Run(ctx context.Context) {
	slog.Info("messaging: sqs consumer started", "queue", c.sqs.wagerQueueURL)
	c.wg.Add(1)
	defer c.wg.Done()
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
			if ctx.Err() != nil {
				// Release visibility so another instance can pick up promptly.
				c.releaseVisibility(context.Background(), aws.ToString(msg.ReceiptHandle))
				continue
			}
			c.wg.Add(1)
			go func(msg types.Message) {
				defer c.wg.Done()
				// Finish in-flight even if parent ctx cancelled (SIGTERM grace).
				c.handle(context.WithoutCancel(ctx), msg)
			}(msg)
		}
	}
}

// Wait blocks until the receive loop and in-flight handlers finish.
func (c *Consumer) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
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
		observability.SQSDLQ.Add(1)
		return
	}

	_, err = c.uc.ProcessFromQueue(ctx, ConsumerWagerTransactions, parsed.MessageID, parsed.PayloadHash, parsed.Input)
	if err == nil {
		c.delete(ctx, receipt)
		return
	}

	switch {
	case errors.Is(err, usecase.ErrPermanent), errors.Is(err, usecase.ErrInvalidInput), errors.Is(err, usecase.ErrConflict):
		slog.Warn("messaging: permanent failure → DLQ",
			"messageId", parsed.MessageID,
			"providerId", parsed.Input.ProviderID,
			"walletId", parsed.Input.WalletID,
			"err", err,
		)
		c.sendDLQ(ctx, body, truncateErr(err))
		c.delete(ctx, receipt)
		observability.SQSDLQ.Add(1)
	default:
		slog.Error("messaging: transient failure",
			"messageId", parsed.MessageID,
			"providerId", parsed.Input.ProviderID,
			"walletId", parsed.Input.WalletID,
			"err", err,
		)
		observability.SQSTransientErrors.Add(1)
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

func (c *Consumer) releaseVisibility(ctx context.Context, receipt string) {
	if receipt == "" {
		return
	}
	_, err := c.sqs.api.ChangeMessageVisibility(ctx, &sqs.ChangeMessageVisibilityInput{
		QueueUrl:          aws.String(c.sqs.wagerQueueURL),
		ReceiptHandle:     aws.String(receipt),
		VisibilityTimeout: 0,
	})
	if err != nil {
		slog.Warn("messaging: release visibility failed", "err", err)
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
	return sha256Hex([]byte(s))
}
