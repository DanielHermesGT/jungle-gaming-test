package messaging_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/config"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/messaging"
)

func TestLocalStackPublishReceive(t *testing.T) {
	if os.Getenv("AWS_ENDPOINT_URL") == "" || os.Getenv("SQS_DOMAIN_EVENTS_QUEUE_URL") == "" {
		t.Skip("AWS_ENDPOINT_URL / SQS_DOMAIN_EVENTS_QUEUE_URL not set; start LocalStack")
	}
	cfg := config.Config{
		AWSRegion:               envOr("AWS_REGION", "us-east-1"),
		AWSEndpointURL:          os.Getenv("AWS_ENDPOINT_URL"),
		AWSAccessKeyID:          envOr("AWS_ACCESS_KEY_ID", "test"),
		AWSSecretAccessKey:      envOr("AWS_SECRET_ACCESS_KEY", "test"),
		SQSWagerQueueURL:        os.Getenv("SQS_WAGER_QUEUE_URL"),
		SQSWagerDLQURL:          os.Getenv("SQS_WAGER_DLQ_URL"),
		SQSDomainEventsQueueURL: os.Getenv("SQS_DOMAIN_EVENTS_QUEUE_URL"),
	}
	if cfg.SQSWagerQueueURL == "" {
		t.Skip("SQS_WAGER_QUEUE_URL not set")
	}

	client, err := messaging.NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.Ready(ctx); err != nil {
		t.Fatalf("sqs ready: %v", err)
	}

	body := `{"eventId":"test-evt-1","eventType":"WagerTransactionProcessed","aggregateId":"w1"}`
	api := exposeAPI(t, client)
	_, err = api.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:               aws.String(cfg.SQSDomainEventsQueueURL),
		MessageBody:            aws.String(body),
		MessageGroupId:         aws.String("w1"),
		MessageDeduplicationId: aws.String("test-evt-1-" + time.Now().Format("150405.000")),
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	out, err := api.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(cfg.SQSDomainEventsQueueURL),
		MaxNumberOfMessages: 1,
		WaitTimeSeconds:     5,
	})
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if len(out.Messages) == 0 {
		t.Fatal("expected at least one message")
	}
	_, _ = api.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(cfg.SQSDomainEventsQueueURL),
		ReceiptHandle: out.Messages[0].ReceiptHandle,
	})
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// exposeAPI uses Ready as smoke; for send/receive we rebuild a client via NewClient internals.
// Prefer testing through exported Ready + a thin test helper in the messaging package.
func exposeAPI(t *testing.T, c *messaging.Client) messaging.SQSAPI {
	t.Helper()
	return messaging.TestAPI(c)
}
