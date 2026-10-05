package database_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/database"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/gateway"
)

func TestOutboxClaimSkipLockedConcurrent(t *testing.T) {
	db := database.OpenTestDB(t)
	ctx := context.Background()
	repo := database.NewOutboxRepo()
	now := time.Date(2026, 10, 4, 16, 0, 0, 0, time.UTC)

	err := db.WithinTx(ctx, func(q gateway.Querier) error {
		for i := 0; i < 4; i++ {
			if err := repo.Insert(ctx, q, gateway.OutboxRecord{
				ID:            fmt.Sprintf("evt-%d", i),
				EventType:     "WagerTransactionProcessed",
				AggregateID:   "agg",
				AggregateType: "WagerTransaction",
				Payload:       json.RawMessage(`{"eventId":"x"}`),
				OccurredAt:    now,
				CreatedAt:     now,
				NextAttemptAt: now,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var a, b []gateway.OutboxRecord
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = db.WithinTx(ctx, func(q gateway.Querier) error {
			var err error
			a, err = repo.ClaimBatch(ctx, q, "worker-a", 2, now)
			time.Sleep(50 * time.Millisecond)
			return err
		})
	}()
	go func() {
		defer wg.Done()
		_ = db.WithinTx(ctx, func(q gateway.Querier) error {
			var err error
			b, err = repo.ClaimBatch(ctx, q, "worker-b", 2, now)
			return err
		})
	}()
	wg.Wait()

	if len(a)+len(b) != 4 {
		t.Fatalf("want 4 claimed total, got a=%d b=%d", len(a), len(b))
	}
	seen := map[string]bool{}
	for _, r := range append(a, b...) {
		if seen[r.ID] {
			t.Fatalf("duplicate claim %s", r.ID)
		}
		seen[r.ID] = true
	}
}

func TestInboxGetInsertComplete(t *testing.T) {
	db := database.OpenTestDB(t)
	ctx := context.Background()
	repo := database.NewInboxRepo()
	now := time.Date(2026, 10, 4, 16, 0, 0, 0, time.UTC)

	err := db.WithinTx(ctx, func(q gateway.Querier) error {
		if err := repo.InsertReceived(ctx, q, gateway.InboxMessage{
			ConsumerName: "wager-transactions",
			MessageID:    "msg-1",
			PayloadHash:  "hash-a",
			ReceivedAt:   now,
		}); err != nil {
			return err
		}
		got, err := repo.Get(ctx, q, "wager-transactions", "msg-1")
		if err != nil || got.CompletedAt != nil || got.PayloadHash != "hash-a" {
			t.Fatalf("got=%+v err=%v", got, err)
		}
		if err := repo.MarkCompleted(ctx, q, "wager-transactions", "msg-1", now); err != nil {
			return err
		}
		got, err = repo.Get(ctx, q, "wager-transactions", "msg-1")
		if err != nil || got.CompletedAt == nil {
			t.Fatalf("completed=%+v err=%v", got, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
