package integration_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/config"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/database"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
	domainwager "github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wager"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/gateway"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/messaging"
	usecasewager "github.com/DanielHermesGT/jungle-gaming-test/internal/usecase/wager"
	usecasewallet "github.com/DanielHermesGT/jungle-gaming-test/internal/usecase/wallet"
	"github.com/DanielHermesGT/jungle-gaming-test/pkg/clock"
	"github.com/DanielHermesGT/jungle-gaming-test/pkg/idgen"
)

// TestOutboxPublishRecoverAfterCrash: claim + publish to SQS without MarkPublished,
// then another worker claims (after lock expiry simulation via MarkRetry unlock) and republishes same eventId.
func TestOutboxPublishRecoverAfterCrash(t *testing.T) {
	if os.Getenv("AWS_ENDPOINT_URL") == "" || os.Getenv("SQS_DOMAIN_EVENTS_QUEUE_URL") == "" {
		t.Skip("LocalStack SQS not configured")
	}
	db := database.OpenTestDB(t)
	ctx := context.Background()
	repo := database.NewOutboxRepo()
	now := time.Now().UTC()
	payload, _ := json.Marshal(map[string]string{"eventId": "recover-1", "eventType": "WagerTransactionProcessed"})
	err := db.WithinTx(ctx, func(q gateway.Querier) error {
		return repo.Insert(ctx, q, gateway.OutboxRecord{
			ID: "recover-1", EventType: "WagerTransactionProcessed",
			AggregateID: "w1", AggregateType: "WagerTransaction",
			Payload: payload, OccurredAt: now, CreatedAt: now, NextAttemptAt: now,
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{
		AWSRegion: envOr("AWS_REGION", "us-east-1"), AWSEndpointURL: os.Getenv("AWS_ENDPOINT_URL"),
		AWSAccessKeyID: envOr("AWS_ACCESS_KEY_ID", "test"), AWSSecretAccessKey: envOr("AWS_SECRET_ACCESS_KEY", "test"),
		SQSWagerQueueURL: os.Getenv("SQS_WAGER_QUEUE_URL"), SQSWagerDLQURL: os.Getenv("SQS_WAGER_DLQ_URL"),
		SQSDomainEventsQueueURL: os.Getenv("SQS_DOMAIN_EVENTS_QUEUE_URL"),
	}
	client, err := messaging.NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	api := messaging.TestAPI(client)

	var claimed []gateway.OutboxRecord
	_ = db.WithinTx(ctx, func(q gateway.Querier) error {
		var err error
		claimed, err = repo.ClaimBatch(ctx, q, "publisher-a", 1, now)
		return err
	})
	if len(claimed) != 1 {
		t.Fatalf("claim=%d", len(claimed))
	}
	_, err = api.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl: aws.String(cfg.SQSDomainEventsQueueURL), MessageBody: aws.String(string(claimed[0].Payload)),
		MessageGroupId: aws.String("w1"), MessageDeduplicationId: aws.String(claimed[0].ID + "-a"),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Crash before MarkPublished: unlock via MarkRetry so another instance can reclaim.
	_ = db.WithinTx(ctx, func(q gateway.Querier) error {
		return repo.MarkRetry(ctx, q, claimed[0].ID, claimed[0].Attempts, now, "crashed before ack")
	})

	var claimed2 []gateway.OutboxRecord
	_ = db.WithinTx(ctx, func(q gateway.Querier) error {
		var err error
		claimed2, err = repo.ClaimBatch(ctx, q, "publisher-b", 1, now)
		return err
	})
	if len(claimed2) != 1 || claimed2[0].ID != "recover-1" {
		t.Fatalf("reclaim=%+v", claimed2)
	}
	_, err = api.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl: aws.String(cfg.SQSDomainEventsQueueURL), MessageBody: aws.String(string(claimed2[0].Payload)),
		MessageGroupId: aws.String("w1"), MessageDeduplicationId: aws.String(claimed2[0].ID + "-b"),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.WithinTx(ctx, func(q gateway.Querier) error {
		return repo.MarkPublished(ctx, q, claimed2[0].ID, time.Now().UTC())
	})
}

func TestProcessFromQueueRedeliveryAfterCommit(t *testing.T) {
	db := database.OpenTestDB(t)
	ctx := context.Background()
	ids := idgen.UUID{}
	clk := clock.System{}
	outbox := database.NewOutboxRepo()
	inbox := database.NewInboxRepo()
	wallets := database.NewWalletRepo()
	ledgers := database.NewLedgerRepo()
	wagers := database.NewWagerRepo()
	wuc := usecasewallet.NewUseCase(db, wallets, ledgers, wagers, outbox, ids, clk)
	guc := usecasewager.NewUseCase(db, wallets, ledgers, wagers, outbox, inbox, ids, clk)

	opened, err := wuc.Open(ctx, usecasewallet.OpenInput{
		PlayerID: "p-redeliver", InitialBalance: mustParse(t, "30.00", "BRL"),
	})
	if err != nil {
		t.Fatal(err)
	}
	amt := mustParse(t, "5.00", "BRL")
	hash, _ := usecasewager.CanonicalPayloadHash(usecasewager.PayloadFields{
		ProviderID: "provider-a", ExternalTransactionID: "redeliver-1",
		PlayerID: "p-redeliver", WalletID: opened.ID, RoundID: "r", GameID: "g",
		Kind: domainwager.KindBet, Amount: amt,
	})
	in := usecasewager.ProcessInput{
		ProviderID: "provider-a", ExternalTransactionID: "redeliver-1",
		IdempotencyKey: "provider-a:redeliver-1", PlayerID: "p-redeliver", WalletID: opened.ID,
		RoundID: "r", GameID: "g", Kind: domainwager.KindBet, Amount: amt, PayloadHash: hash,
	}
	body := []byte(`{"messageId":"msg-redeliver","data":{}}`)
	sum := sha256.Sum256(body)
	transport := hex.EncodeToString(sum[:])

	first, err := guc.ProcessFromQueue(ctx, messaging.ConsumerWagerTransactions, "msg-redeliver", transport, in)
	if err != nil || first.IdempotentReplay {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	// Simulate: commit done, SQS delete never happened → redelivery.
	second, err := guc.ProcessFromQueue(ctx, messaging.ConsumerWagerTransactions, "msg-redeliver", transport, in)
	if err != nil || !second.IdempotentReplay {
		t.Fatalf("redelivery=%+v err=%v", second, err)
	}
	w, _ := wuc.Get(ctx, opened.ID)
	if w.Balance.AmountString() != "25.00" {
		t.Fatalf("balance=%s", w.Balance.AmountString())
	}
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func mustParse(t *testing.T, amount, currency string) money.Money {
	t.Helper()
	m, err := money.Parse(currency, amount)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
