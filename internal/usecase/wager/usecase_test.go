package wager_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
	domainwager "github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wager"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/usecase"
	usecasewager "github.com/DanielHermesGT/jungle-gaming-test/internal/usecase/wager"
	usecasewallet "github.com/DanielHermesGT/jungle-gaming-test/internal/usecase/wallet"
)

func TestProcessBetWinLossAndIdempotency(t *testing.T) {
	wuc, guc, _ := newUseCases(t)
	ctx := context.Background()

	opened, err := wuc.Open(ctx, usecasewallet.OpenInput{
		PlayerID: "p-bet", InitialBalance: mustParse(t, "100.00", "BRL"),
	})
	if err != nil {
		t.Fatal(err)
	}

	betIn := processIn(t, opened.ID, "p-bet", domainwager.KindBet, "25.00", "bet-1", "")
	bet, err := guc.Process(ctx, betIn)
	if err != nil {
		t.Fatal(err)
	}
	if bet.Transaction.Status() != domainwager.StatusProcessed || bet.Balance.AmountString() != "75.00" {
		t.Fatalf("bet=%+v", bet)
	}

	replay, err := guc.Process(ctx, betIn)
	if err != nil || !replay.IdempotentReplay || replay.Balance.AmountString() != "75.00" {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}

	conflict := betIn
	conflict.Amount = mustParse(t, "30.00", "BRL")
	hash, _ := usecasewager.CanonicalPayloadHash(usecasewager.PayloadFields{
		ProviderID: conflict.ProviderID, ExternalTransactionID: conflict.ExternalTransactionID,
		PlayerID: conflict.PlayerID, WalletID: conflict.WalletID, RoundID: conflict.RoundID,
		GameID: conflict.GameID, Kind: conflict.Kind, Amount: conflict.Amount,
	})
	conflict.PayloadHash = hash
	if _, err := guc.Process(ctx, conflict); !errors.Is(err, usecase.ErrConflict) {
		t.Fatalf("want conflict, got %v", err)
	}

	otherKey := betIn
	otherKey.IdempotencyKey = "other-key"
	otherKey.PayloadHash = betIn.PayloadHash
	if _, err := guc.Process(ctx, otherKey); !errors.Is(err, usecase.ErrConflict) {
		t.Fatalf("want external conflict, got %v", err)
	}

	poor := processIn(t, opened.ID, "p-bet", domainwager.KindBet, "100.00", "bet-poor", "")
	rej, err := guc.Process(ctx, poor)
	if err != nil {
		t.Fatal(err)
	}
	if rej.Transaction.Status() != domainwager.StatusRejected ||
		rej.Transaction.FailureCode() != domainwager.FailureInsufficientFunds {
		t.Fatalf("reject=%+v", rej)
	}

	win := processIn(t, opened.ID, "p-bet", domainwager.KindWin, "10.00", "win-1", "")
	won, err := guc.Process(ctx, win)
	if err != nil || won.Balance.AmountString() != "85.00" {
		t.Fatalf("win=%+v err=%v", won, err)
	}

	loss := processIn(t, opened.ID, "p-bet", domainwager.KindLoss, "0.00", "loss-1", "")
	lost, err := guc.Process(ctx, loss)
	if err != nil || lost.Transaction.Status() != domainwager.StatusProcessed {
		t.Fatalf("loss=%+v err=%v", lost, err)
	}
	w, err := wuc.Get(ctx, opened.ID)
	if err != nil || w.Balance.AmountString() != "85.00" || w.Version != 3 {
		// open v1, bet debit v2, win credit v3; loss must not bump version
		t.Fatalf("wallet after loss=%+v", w)
	}
}

func TestRefundPendingReferenceAndResume(t *testing.T) {
	wuc, guc, clk := newUseCases(t)
	ctx := context.Background()

	opened, err := wuc.Open(ctx, usecasewallet.OpenInput{
		PlayerID: "p-ref", InitialBalance: mustParse(t, "50.00", "BRL"),
	})
	if err != nil {
		t.Fatal(err)
	}

	refundEarly := processIn(t, opened.ID, "p-ref", domainwager.KindRefund, "20.00", "refund-1", "bet-late")
	pending, err := guc.Process(ctx, refundEarly)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Transaction.Status() != domainwager.StatusPendingReference {
		t.Fatalf("want PENDING_REFERENCE, got %s", pending.Transaction.Status())
	}

	bet := processIn(t, opened.ID, "p-ref", domainwager.KindBet, "20.00", "bet-late", "")
	if _, err := guc.Process(ctx, bet); err != nil {
		t.Fatal(err)
	}

	resumed, err := guc.ResumePendingReference(ctx, pending.Transaction.ID())
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Transaction.Status() != domainwager.StatusProcessed || resumed.Balance.AmountString() != "50.00" {
		t.Fatalf("resumed=%+v", resumed)
	}

	// TTL expiry path
	refund2 := processIn(t, opened.ID, "p-ref", domainwager.KindRefund, "20.00", "refund-ttl", "never-arrives")
	pend2, err := guc.Process(ctx, refund2)
	if err != nil {
		t.Fatal(err)
	}
	clk.at = clk.at.Add(usecasewager.PendingReferenceTTL + time.Minute)
	expired, err := guc.ResumePendingReference(ctx, pend2.Transaction.ID())
	if err != nil {
		t.Fatal(err)
	}
	if expired.Transaction.Status() != domainwager.StatusRejected ||
		expired.Transaction.FailureCode() != domainwager.FailureReferenceNotFound {
		t.Fatalf("expired=%+v", expired)
	}
}

func TestConcurrentBetsOneFails(t *testing.T) {
	wuc, guc, _ := newUseCases(t)
	ctx := context.Background()
	opened, err := wuc.Open(ctx, usecasewallet.OpenInput{
		PlayerID: "p-race", InitialBalance: mustParse(t, "100.00", "BRL"),
	})
	if err != nil {
		t.Fatal(err)
	}

	inputs := []usecasewager.ProcessInput{
		processIn(t, opened.ID, "p-race", domainwager.KindBet, "80.00", "race-a", ""),
		processIn(t, opened.ID, "p-race", domainwager.KindBet, "80.00", "race-b", ""),
	}
	inputs[0].IdempotencyKey = "race-key-0"
	inputs[1].IdempotencyKey = "race-key-1"

	var wg sync.WaitGroup
	results := make(chan domainwager.Status, 2)
	for _, in := range inputs {
		wg.Add(1)
		go func(in usecasewager.ProcessInput) {
			defer wg.Done()
			out, err := guc.Process(ctx, in)
			if err != nil {
				results <- domainwager.Status("ERR")
				return
			}
			results <- out.Transaction.Status()
		}(in)
	}
	wg.Wait()
	close(results)

	var processed, rejected int
	for st := range results {
		switch st {
		case domainwager.StatusProcessed:
			processed++
		case domainwager.StatusRejected:
			rejected++
		}
	}
	if processed != 1 || rejected != 1 {
		t.Fatalf("processed=%d rejected=%d", processed, rejected)
	}
}

func processIn(
	t *testing.T,
	walletID, playerID string,
	kind domainwager.Kind,
	amount, external, ref string,
) usecasewager.ProcessInput {
	t.Helper()
	amt := mustParse(t, amount, "BRL")
	in := usecasewager.ProcessInput{
		ProviderID:                     "prov-a",
		ExternalTransactionID:          external,
		IdempotencyKey:                 "prov-a:" + external,
		PlayerID:                       playerID,
		WalletID:                       walletID,
		RoundID:                        "round-1",
		GameID:                         "game-1",
		Kind:                           kind,
		Amount:                         amt,
		ReferenceExternalTransactionID: ref,
	}
	hash, err := usecasewager.CanonicalPayloadHash(usecasewager.PayloadFields{
		ProviderID: in.ProviderID, ExternalTransactionID: in.ExternalTransactionID,
		PlayerID: in.PlayerID, WalletID: in.WalletID, RoundID: in.RoundID,
		GameID: in.GameID, Kind: in.Kind, Amount: in.Amount,
		ReferenceExternalTransactionID: ref,
	})
	if err != nil {
		t.Fatal(err)
	}
	in.PayloadHash = hash
	return in
}

func newUseCases(t *testing.T) (*usecasewallet.UseCase, *usecasewager.UseCase, *fixedClock) {
	t.Helper()
	wuc, guc, clk, _ := newUseCasesDB(t)
	return wuc, guc, clk
}

type seqIDs struct{ n int }

func (s *seqIDs) New() string {
	s.n++
	return fmt.Sprintf("id-%d", s.n)
}

type fixedClock struct{ at time.Time }

func (c *fixedClock) Now() time.Time { return c.at }

func mustParse(t *testing.T, amount, currency string) money.Money {
	t.Helper()
	m, err := money.Parse(currency, amount)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
