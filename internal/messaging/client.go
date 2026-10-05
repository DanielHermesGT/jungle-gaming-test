package messaging

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/config"
)

// SQSAPI is the subset of SQS used by workers and health checks.
type SQSAPI interface {
	ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
	SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
	GetQueueAttributes(ctx context.Context, params *sqs.GetQueueAttributesInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error)
}

// Client wraps the AWS SQS client and configured queue URLs.
type Client struct {
	api            SQSAPI
	wagerQueueURL  string
	wagerDLQURL    string
	eventsQueueURL string
}

func NewClient(cfg config.Config) (*Client, error) {
	loadOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.AWSRegion),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AWSAccessKeyID, cfg.AWSSecretAccessKey, "",
		)),
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("messaging: aws config: %w", err)
	}

	api := sqs.NewFromConfig(awsCfg, func(o *sqs.Options) {
		if cfg.AWSEndpointURL != "" {
			o.BaseEndpoint = aws.String(cfg.AWSEndpointURL)
		}
	})

	return &Client{
		api:            api,
		wagerQueueURL:  cfg.SQSWagerQueueURL,
		wagerDLQURL:    cfg.SQSWagerDLQURL,
		eventsQueueURL: cfg.SQSDomainEventsQueueURL,
	}, nil
}

// Ready pings the wager input queue (readiness for SQS).
func (c *Client) Ready(ctx context.Context) error {
	_, err := c.api.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(c.wagerQueueURL),
		AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameApproximateNumberOfMessages},
	})
	if err != nil {
		return fmt.Errorf("messaging: sqs ready: %w", err)
	}
	return nil
}

// TestAPI exposes the underlying SQS API for integration tests.
func TestAPI(c *Client) SQSAPI { return c.api }
