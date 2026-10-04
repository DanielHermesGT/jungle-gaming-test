package wager_test

import (
	"errors"
	"testing"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wager"
)

func mustMoney(t *testing.T, minor int64) money.Money {
	t.Helper()
	m, err := money.FromMinor(minor, "BRL")
	if err != nil {
		t.Fatalf("money: %v", err)
	}
	return m
}

func TestParseKind(t *testing.T) {
	kinds := []string{"OPENING", "BET", "WIN", "LOSS", "REFUND", "ROLLBACK"}
	for _, s := range kinds {
		k, err := wager.ParseKind(s)
		if err != nil || string(k) != s {
			t.Fatalf("ParseKind(%q) = %q, %v", s, k, err)
		}
	}
	if _, err := wager.ParseKind("FOO"); !errors.Is(err, wager.ErrInvalidKind) {
		t.Fatalf("want ErrInvalidKind, got %v", err)
	}
}

func TestAmountPolicies(t *testing.T) {
	now := time.Now().UTC()
	base := wager.ExternalParams{
		ID:                    "tx-1",
		WalletID:              "w-1",
		PlayerID:              "p-1",
		ProviderID:            "prov-a",
		ExternalTransactionID: "ext-1",
		IdempotencyKey:        "idem-1",
		PayloadHash:           "hash-1",
		RoundID:               "r-1",
		GameID:                "g-1",
		Now:                   now,
	}

	t.Run("LOSS zero ok", func(t *testing.T) {
		p := base
		p.Kind = wager.KindLoss
		p.Amount = mustMoney(t, 0)
		if _, err := wager.NewExternal(p); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})
	t.Run("LOSS non-zero rejected", func(t *testing.T) {
		p := base
		p.Kind = wager.KindLoss
		p.Amount = mustMoney(t, 100)
		if _, err := wager.NewExternal(p); !errors.Is(err, wager.ErrInvalidAmount) {
			t.Fatalf("want ErrInvalidAmount, got %v", err)
		}
	})
	t.Run("BET positive ok", func(t *testing.T) {
		p := base
		p.Kind = wager.KindBet
		p.Amount = mustMoney(t, 100)
		if _, err := wager.NewExternal(p); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})
	t.Run("BET zero rejected", func(t *testing.T) {
		p := base
		p.Kind = wager.KindBet
		p.Amount = mustMoney(t, 0)
		if _, err := wager.NewExternal(p); !errors.Is(err, wager.ErrInvalidAmount) {
			t.Fatalf("want ErrInvalidAmount, got %v", err)
		}
	})
	t.Run("OPENING zero ok", func(t *testing.T) {
		tx, err := wager.NewOpening(wager.OpeningParams{
			ID: "o-1", WalletID: "w-1", PlayerID: "p-1", Amount: mustMoney(t, 0), Now: now,
		})
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if tx.Status() != wager.StatusProcessed || tx.Origin() != wager.OriginInternal {
			t.Fatalf("status=%s origin=%s", tx.Status(), tx.Origin())
		}
	})
	t.Run("OPENING negative rejected", func(t *testing.T) {
		neg, _ := money.FromMinor(-1, "BRL")
		_, err := wager.NewOpening(wager.OpeningParams{
			ID: "o-1", WalletID: "w-1", PlayerID: "p-1", Amount: neg, Now: now,
		})
		if !errors.Is(err, wager.ErrInvalidAmount) {
			t.Fatalf("want ErrInvalidAmount, got %v", err)
		}
	})
}

func TestNewExternalRejectsOpening(t *testing.T) {
	_, err := wager.NewExternal(wager.ExternalParams{
		ID: "tx-1", Kind: wager.KindOpening, WalletID: "w-1", PlayerID: "p-1",
		ProviderID: "prov", ExternalTransactionID: "e", IdempotencyKey: "i",
		PayloadHash: "h", RoundID: "r", GameID: "g", Amount: mustMoney(t, 100), Now: time.Now().UTC(),
	})
	if !errors.Is(err, wager.ErrInvalidKind) {
		t.Fatalf("want ErrInvalidKind, got %v", err)
	}
}

func TestNewOpeningHasNoExternalFields(t *testing.T) {
	tx, err := wager.NewOpening(wager.OpeningParams{
		ID: "o-1", WalletID: "w-1", PlayerID: "p-1", Amount: mustMoney(t, 500), Now: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if tx.ProviderID() != "" || tx.ExternalTransactionID() != "" || tx.IdempotencyKey() != "" {
		t.Fatalf("external fields must be empty on OPENING")
	}
	if tx.Kind() != wager.KindOpening || tx.Status() != wager.StatusProcessed {
		t.Fatalf("kind=%s status=%s", tx.Kind(), tx.Status())
	}
}

func TestRefundRollbackRequireReference(t *testing.T) {
	now := time.Now().UTC()
	base := wager.ExternalParams{
		ID: "tx-1", WalletID: "w-1", PlayerID: "p-1",
		ProviderID: "prov", ExternalTransactionID: "e", IdempotencyKey: "i",
		PayloadHash: "h", RoundID: "r", GameID: "g", Amount: mustMoney(t, 100), Now: now,
	}
	for _, kind := range []wager.Kind{wager.KindRefund, wager.KindRollback} {
		p := base
		p.Kind = kind
		if _, err := wager.NewExternal(p); !errors.Is(err, wager.ErrMissingReference) {
			t.Fatalf("%s without ref: want ErrMissingReference, got %v", kind, err)
		}
		p.ReferenceExternalTransaction = "bet-ext-1"
		if _, err := wager.NewExternal(p); err != nil {
			t.Fatalf("%s with ref: unexpected %v", kind, err)
		}
	}
}

func TestFSMHappyPathAndTerminalBlock(t *testing.T) {
	now := time.Now().UTC()
	tx, err := wager.NewExternal(wager.ExternalParams{
		ID: "tx-1", Kind: wager.KindBet, WalletID: "w-1", PlayerID: "p-1",
		ProviderID: "prov", ExternalTransactionID: "e", IdempotencyKey: "i",
		PayloadHash: "h", RoundID: "r", GameID: "g", Amount: mustMoney(t, 100), Now: now,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	awaiting, err := tx.AwaitReference(now.Add(time.Second))
	if err != nil || awaiting.Status() != wager.StatusPendingReference {
		t.Fatalf("AwaitReference: status=%s err=%v", awaiting.Status(), err)
	}

	bal := mustMoney(t, 900)
	processed, err := awaiting.MarkProcessed(&bal, now.Add(2*time.Second))
	if err != nil || processed.Status() != wager.StatusProcessed {
		t.Fatalf("MarkProcessed: status=%s err=%v", processed.Status(), err)
	}
	if processed.ResultBalance() == nil || processed.ResultBalance().Minor() != 900 {
		t.Fatalf("result balance not set")
	}

	if _, err := processed.MarkRejected(wager.FailureInsufficientFunds, now); !errors.Is(err, wager.ErrTerminalStatus) {
		t.Fatalf("terminal reject: want ErrTerminalStatus, got %v", err)
	}
	if _, err := processed.AwaitReference(now); !errors.Is(err, wager.ErrTerminalStatus) {
		t.Fatalf("terminal await: want ErrTerminalStatus, got %v", err)
	}
}

func TestMarkRejectedAndFailed(t *testing.T) {
	now := time.Now().UTC()
	tx, err := wager.NewExternal(wager.ExternalParams{
		ID: "tx-1", Kind: wager.KindWin, WalletID: "w-1", PlayerID: "p-1",
		ProviderID: "prov", ExternalTransactionID: "e", IdempotencyKey: "i",
		PayloadHash: "h", RoundID: "r", GameID: "g", Amount: mustMoney(t, 50), Now: now,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	rejected, err := tx.MarkRejected(wager.FailureInvalidAmount, now.Add(time.Second))
	if err != nil || rejected.Status() != wager.StatusRejected {
		t.Fatalf("reject: status=%s err=%v", rejected.Status(), err)
	}
	if rejected.FailureCode() != wager.FailureInvalidAmount {
		t.Fatalf("failure code=%s", rejected.FailureCode())
	}

	tx2, _ := wager.NewExternal(wager.ExternalParams{
		ID: "tx-2", Kind: wager.KindBet, WalletID: "w-1", PlayerID: "p-1",
		ProviderID: "prov", ExternalTransactionID: "e2", IdempotencyKey: "i2",
		PayloadHash: "h", RoundID: "r", GameID: "g", Amount: mustMoney(t, 50), Now: now,
	})
	failed, err := tx2.MarkFailed(wager.FailureInvalidTransition, now)
	if err != nil || failed.Status() != wager.StatusFailed {
		t.Fatalf("fail: status=%s err=%v", failed.Status(), err)
	}
}

func TestResolveReference(t *testing.T) {
	now := time.Now().UTC()
	tx, err := wager.NewExternal(wager.ExternalParams{
		ID: "tx-1", Kind: wager.KindRefund, WalletID: "w-1", PlayerID: "p-1",
		ProviderID: "prov", ExternalTransactionID: "e", IdempotencyKey: "i",
		PayloadHash: "h", RoundID: "r", GameID: "g", Amount: mustMoney(t, 100),
		ReferenceExternalTransaction: "bet-ext", Now: now,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	resolved, err := tx.ResolveReference("internal-bet-id", now.Add(time.Second))
	if err != nil || resolved.ResolvedReferenceID() != "internal-bet-id" {
		t.Fatalf("resolve: id=%s err=%v", resolved.ResolvedReferenceID(), err)
	}
	processed, _ := resolved.MarkProcessed(nil, now.Add(2*time.Second))
	if _, err := processed.ResolveReference("x", now); !errors.Is(err, wager.ErrTerminalStatus) {
		t.Fatalf("want ErrTerminalStatus, got %v", err)
	}
}

func TestFromPersistedPreservesState(t *testing.T) {
	now := time.Now().UTC()
	bal := mustMoney(t, 42)
	tx, err := wager.FromPersisted(wager.PersistedParams{
		ID: "tx-1", Origin: wager.OriginExternal, Kind: wager.KindBet,
		Status: wager.StatusPendingReference, WalletID: "w-1", PlayerID: "p-1",
		Amount: mustMoney(t, 100), CreatedAt: now, UpdatedAt: now.Add(time.Second),
		ProviderID: "prov", ExternalTransactionID: "e", IdempotencyKey: "i",
		PayloadHash: "h", RoundID: "r", GameID: "g",
		ResolvedReferenceID: "ref-internal", ResultBalance: &bal,
	})
	if err != nil {
		t.Fatalf("from persisted: %v", err)
	}
	if tx.Status() != wager.StatusPendingReference {
		t.Fatalf("status=%s", tx.Status())
	}
	if tx.ResolvedReferenceID() != "ref-internal" {
		t.Fatalf("resolved=%s", tx.ResolvedReferenceID())
	}
	if tx.ResultBalance() == nil || tx.ResultBalance().Minor() != 42 {
		t.Fatalf("result balance lost")
	}

	opening, err := wager.FromPersisted(wager.PersistedParams{
		ID: "o-1", Origin: wager.OriginInternal, Kind: wager.KindOpening,
		Status: wager.StatusProcessed, WalletID: "w-1", PlayerID: "p-1",
		Amount: mustMoney(t, 0), CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("opening from persisted: %v", err)
	}
	if opening.Origin() != wager.OriginInternal || opening.Kind() != wager.KindOpening {
		t.Fatalf("opening fields wrong")
	}
}

func TestStatusIsTerminal(t *testing.T) {
	if !wager.StatusProcessed.IsTerminal() || !wager.StatusRejected.IsTerminal() || !wager.StatusFailed.IsTerminal() {
		t.Fatal("terminal statuses")
	}
	if wager.StatusPending.IsTerminal() || wager.StatusPendingReference.IsTerminal() {
		t.Fatal("non-terminal must not be terminal")
	}
}
