package wager_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/database"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/event"
	domainwager "github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wager"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/usecase"
	usecasewager "github.com/DanielHermesGT/jungle-gaming-test/internal/usecase/wager"
	usecasewallet "github.com/DanielHermesGT/jungle-gaming-test/internal/usecase/wallet"
)

func TestOpenOutboxEvents(t *testing.T) {
	wuc, _, _, db := newUseCasesDB(t)
	ctx := context.Background()

	_, err := wuc.Open(ctx, usecasewallet.OpenInput{
		PlayerID: "p-outbox-pos", InitialBalance: mustParse(t, "10.00", "BRL"),
	})
	if err != nil {
		t.Fatal(err)
	}
	types := allOutboxTypes(t, db)
	if len(types) != 2 || !contains(types, event.TypeWagerProcessed) || !contains(types, event.TypeWalletBalanceChanged) {
		t.Fatalf("open>0 types=%v", types)
	}

	before := len(types)
	_, err = wuc.Open(ctx, usecasewallet.OpenInput{
		PlayerID: "p-outbox-zero", InitialBalance: mustParse(t, "0.00", "USD"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(allOutboxTypes(t, db)); n != before {
		t.Fatalf("open 0 must not add outbox, before=%d after=%d", before, n)
	}
}

func TestProcessOutboxByOutcome(t *testing.T) {
	wuc, guc, _, db := newUseCasesDB(t)
	ctx := context.Background()

	opened, err := wuc.Open(ctx, usecasewallet.OpenInput{
		PlayerID: "p-out-proc", InitialBalance: mustParse(t, "50.00", "BRL"),
	})
	if err != nil {
		t.Fatal(err)
	}

	bet := processIn(t, opened.ID, "p-out-proc", domainwager.KindBet, "10.00", "ob-bet", "")
	out, err := guc.Process(ctx, bet)
	if err != nil {
		t.Fatal(err)
	}
	types := queryOutboxTypes(t, db, out.Transaction.ID())
	if !contains(types, event.TypeWagerProcessed) || !contains(types, event.TypeWalletBalanceChanged) {
		t.Fatalf("bet types=%v", types)
	}

	loss := processIn(t, opened.ID, "p-out-proc", domainwager.KindLoss, "0.00", "ob-loss", "")
	lost, err := guc.Process(ctx, loss)
	if err != nil {
		t.Fatal(err)
	}
	types = queryOutboxTypes(t, db, lost.Transaction.ID())
	if len(types) != 1 || types[0] != event.TypeWagerProcessed {
		t.Fatalf("loss types=%v", types)
	}

	poor := processIn(t, opened.ID, "p-out-proc", domainwager.KindBet, "100.00", "ob-poor", "")
	rej, err := guc.Process(ctx, poor)
	if err != nil {
		t.Fatal(err)
	}
	types = queryOutboxTypes(t, db, rej.Transaction.ID())
	if len(types) != 1 || types[0] != event.TypeWagerRejected {
		t.Fatalf("reject types=%v", types)
	}
}

func TestProcessFromQueueInboxReplay(t *testing.T) {
	wuc, guc, _, _ := newUseCasesDB(t)
	ctx := context.Background()

	opened, err := wuc.Open(ctx, usecasewallet.OpenInput{
		PlayerID: "p-inbox", InitialBalance: mustParse(t, "40.00", "BRL"),
	})
	if err != nil {
		t.Fatal(err)
	}
	in := processIn(t, opened.ID, "p-inbox", domainwager.KindBet, "5.00", "inbox-bet", "")
	transportHash := "transport-hash-1"

	first, err := guc.ProcessFromQueue(ctx, "wager-transactions", "msg-inbox-1", transportHash, in)
	if err != nil || first.IdempotentReplay {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	bal := first.Balance.AmountString()

	second, err := guc.ProcessFromQueue(ctx, "wager-transactions", "msg-inbox-1", transportHash, in)
	if err != nil || !second.IdempotentReplay || second.Balance.AmountString() != bal {
		t.Fatalf("replay=%+v err=%v", second, err)
	}

	_, err = guc.ProcessFromQueue(ctx, "wager-transactions", "msg-inbox-1", "other-hash", in)
	if !errors.Is(err, usecase.ErrPermanent) {
		t.Fatalf("want permanent hash mismatch, got %v", err)
	}
}

func TestPendingReferenceOutboxAndResumeRejected(t *testing.T) {
	wuc, guc, clk, db := newUseCasesDB(t)
	ctx := context.Background()

	opened, err := wuc.Open(ctx, usecasewallet.OpenInput{
		PlayerID: "p-pref", InitialBalance: mustParse(t, "30.00", "BRL"),
	})
	if err != nil {
		t.Fatal(err)
	}

	refund := processIn(t, opened.ID, "p-pref", domainwager.KindRefund, "10.00", "pref-1", "missing-bet")
	pending, err := guc.Process(ctx, refund)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Transaction.Status() != domainwager.StatusPendingReference {
		t.Fatalf("status=%s", pending.Transaction.Status())
	}
	types := queryOutboxTypes(t, db, pending.Transaction.ID())
	if len(types) != 1 || types[0] != event.TypeWagerPendingReference {
		t.Fatalf("pending types=%v", types)
	}

	clk.at = clk.at.Add(usecasewager.PendingReferenceTTL + time.Minute)
	resumed, err := guc.ResumePendingReference(ctx, pending.Transaction.ID())
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Transaction.Status() != domainwager.StatusRejected {
		t.Fatalf("resume=%+v", resumed)
	}
	types = queryOutboxTypes(t, db, pending.Transaction.ID())
	if !contains(types, event.TypeWagerRejected) {
		t.Fatalf("after resume types=%v", types)
	}
}

func newUseCasesDB(t *testing.T) (*usecasewallet.UseCase, *usecasewager.UseCase, *fixedClock, *database.DB) {
	t.Helper()
	db := database.OpenTestDB(t)
	ids := &seqIDs{}
	clk := &fixedClock{at: time.Date(2026, 10, 4, 15, 0, 0, 0, time.UTC)}
	wallets := database.NewWalletRepo()
	ledgers := database.NewLedgerRepo()
	wagers := database.NewWagerRepo()
	outbox := database.NewOutboxRepo()
	inbox := database.NewInboxRepo()
	wuc := usecasewallet.NewUseCase(db, wallets, ledgers, wagers, outbox, ids, clk)
	guc := usecasewager.NewUseCase(db, wallets, ledgers, wagers, outbox, inbox, ids, clk)
	return wuc, guc, clk, db
}

func queryOutboxTypes(t *testing.T, db *database.DB, correlationOrAggregate string) []string {
	t.Helper()
	// Wager events use aggregate_id=txId; WalletBalanceChanged uses aggregate_id=walletId + correlationId=txId.
	return scanOutboxTypes(t, db, `
SELECT event_type FROM outbox_events
WHERE aggregate_id = $1 OR headers->>'correlationId' = $1
ORDER BY created_at, id`, correlationOrAggregate)
}

func allOutboxTypes(t *testing.T, db *database.DB) []string {
	t.Helper()
	return scanOutboxTypes(t, db, `SELECT event_type FROM outbox_events ORDER BY created_at, id`)
}

func scanOutboxTypes(t *testing.T, db *database.DB, sql string, args ...any) []string {
	t.Helper()
	rows, err := db.Pool.Query(context.Background(), sql, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var typ string
		if err := rows.Scan(&typ); err != nil {
			t.Fatal(err)
		}
		out = append(out, typ)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
