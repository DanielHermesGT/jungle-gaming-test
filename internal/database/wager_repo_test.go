package database_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/database"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wager"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wallet"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/gateway"
)

func seedWallet(t *testing.T, db *database.DB, walletID, playerID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	opened, err := wallet.Open(wallet.OpenParams{
		WalletID:       walletID,
		PlayerID:       playerID,
		InitialBalance: mustParse(t, "100.00", "BRL"),
		OpeningTxID:    walletID + "-open-tx",
		LedgerEntryID:  walletID + "-open-ledger",
		Now:            now,
	})
	if err != nil {
		t.Fatal(err)
	}
	wallets := database.NewWalletRepo()
	ledgers := database.NewLedgerRepo()
	err = db.WithinTx(ctx, func(q gateway.Querier) error {
		if err := wallets.Insert(ctx, q, opened.Wallet); err != nil {
			return err
		}
		return ledgers.Insert(ctx, q, *opened.Ledger)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWagerRepoInsertOpeningAndExternal(t *testing.T) {
	db := database.OpenTestDB(t)
	ctx := context.Background()
	seedWallet(t, db, "w-1", "p-1")
	repo := database.NewWagerRepo()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	amount := mustParse(t, "100.00", "BRL")

	opening, err := wager.NewOpening(wager.OpeningParams{
		ID: "open-1", WalletID: "w-1", PlayerID: "p-1", Amount: amount, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Insert(ctx, db.Pool, opening); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByID(ctx, db.Pool, "open-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind() != wager.KindOpening || got.Status() != wager.StatusProcessed {
		t.Fatalf("got=%s %s", got.Kind(), got.Status())
	}

	dup, _ := wager.NewOpening(wager.OpeningParams{
		ID: "open-2", WalletID: "w-1", PlayerID: "p-1", Amount: amount, Now: now,
	})
	if err := repo.Insert(ctx, db.Pool, dup); !errors.Is(err, database.ErrConflict) {
		t.Fatalf("want conflict opening, got %v", err)
	}

	ext, err := wager.NewExternal(wager.ExternalParams{
		ID: "ext-1", Kind: wager.KindBet, WalletID: "w-1", PlayerID: "p-1",
		ProviderID: "prov", ExternalTransactionID: "e-1", IdempotencyKey: "idem-1",
		PayloadHash: "hash-1", RoundID: "r-1", GameID: "g-1", Amount: mustParse(t, "10.00", "BRL"), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	bal := mustParse(t, "90.00", "BRL")
	processed, err := ext.MarkProcessed(&bal, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Insert(ctx, db.Pool, processed); err != nil {
		t.Fatal(err)
	}

	byKey, err := repo.GetByIdempotencyKey(ctx, db.Pool, "idem-1")
	if err != nil || byKey.ID() != "ext-1" {
		t.Fatalf("by key: %+v %v", byKey, err)
	}
	byExt, err := repo.GetByProviderExternal(ctx, db.Pool, "prov", "e-1")
	if err != nil || byExt.ID() != "ext-1" {
		t.Fatalf("by ext: %+v %v", byExt, err)
	}

	dupExt, _ := wager.NewExternal(wager.ExternalParams{
		ID: "ext-2", Kind: wager.KindBet, WalletID: "w-1", PlayerID: "p-1",
		ProviderID: "prov", ExternalTransactionID: "e-1", IdempotencyKey: "idem-2",
		PayloadHash: "hash-2", RoundID: "r-1", GameID: "g-1", Amount: mustParse(t, "10.00", "BRL"), Now: now,
	})
	if err := repo.Insert(ctx, db.Pool, dupExt); !errors.Is(err, database.ErrConflict) {
		t.Fatalf("want conflict external, got %v", err)
	}
}

func TestWagerRepoUpdateAndPendingReference(t *testing.T) {
	db := database.OpenTestDB(t)
	ctx := context.Background()
	seedWallet(t, db, "w-2", "p-2")
	repo := database.NewWagerRepo()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

	tx, err := wager.NewExternal(wager.ExternalParams{
		ID: "ref-1", Kind: wager.KindRefund, WalletID: "w-2", PlayerID: "p-2",
		ProviderID: "prov", ExternalTransactionID: "refund-1", IdempotencyKey: "idem-r",
		PayloadHash: "h", RoundID: "r", GameID: "g", Amount: mustParse(t, "10.00", "BRL"),
		ReferenceExternalTransaction: "missing-bet", Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	until := now.Add(15 * time.Minute)
	pending, err := tx.AwaitReference(until, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Insert(ctx, db.Pool, pending); err != nil {
		t.Fatal(err)
	}

	due, err := repo.ListPendingReferenceDue(ctx, db.Pool, until, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].ID() != "ref-1" {
		t.Fatalf("due=%+v", due)
	}

	rejected, err := pending.MarkRejected(wager.FailureReferenceNotFound, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(ctx, db.Pool, rejected); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByID(ctx, db.Pool, "ref-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status() != wager.StatusRejected || got.FailureCode() != wager.FailureReferenceNotFound {
		t.Fatalf("got status=%s code=%s", got.Status(), got.FailureCode())
	}
	if !got.PendingReferenceUntil().IsZero() {
		t.Fatalf("until should be cleared")
	}
}
