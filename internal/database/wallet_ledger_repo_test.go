package database_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/database"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wallet"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/gateway"
)

func TestInsertWalletAndLedgerSameTx(t *testing.T) {
	db := database.OpenTestDB(t)
	ctx := context.Background()
	wallets := database.NewWalletRepo()
	ledgers := database.NewLedgerRepo()
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

	opened, err := wallet.Open(wallet.OpenParams{
		WalletID:       "wallet-1",
		PlayerID:       "player-1",
		InitialBalance: mustParse(t, "100.00", "BRL"),
		OpeningTxID:    "tx-open",
		LedgerEntryID:  "ledger-open",
		Now:            now,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Atomicidade: wallet + ledger no mesmo Commit.
	err = db.WithinTx(ctx, func(q gateway.Querier) error {
		if err := wallets.Insert(ctx, q, opened.Wallet); err != nil {
			return err
		}
		return ledgers.Insert(ctx, q, *opened.Ledger)
	})
	if err != nil {
		t.Fatalf("persist: %v", err)
	}

	got, err := wallets.GetByID(ctx, db.Pool, "wallet-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Balance().AmountString() != "100.00" || got.Version() != 1 {
		t.Fatalf("wallet=%s v=%d", got.Balance().AmountString(), got.Version())
	}

	entries, err := ledgers.ListByWalletID(ctx, db.Pool, "wallet-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Direction() != wallet.DirectionCredit {
		t.Fatalf("ledger=%+v", entries)
	}
}

func TestInsertWalletConflictSamePlayerCurrency(t *testing.T) {
	db := database.OpenTestDB(t)
	ctx := context.Background()
	wallets := database.NewWalletRepo()
	now := time.Unix(1, 0).UTC()

	first, err := wallet.Open(wallet.OpenParams{
		WalletID:       "wallet-a",
		PlayerID:       "player-x",
		InitialBalance: mustParse(t, "0.00", "BRL"),
		Now:            now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := wallets.Insert(ctx, db.Pool, first.Wallet); err != nil {
		t.Fatal(err)
	}

	second, err := wallet.Open(wallet.OpenParams{
		WalletID:       "wallet-b",
		PlayerID:       "player-x",
		InitialBalance: mustParse(t, "0.00", "BRL"),
		Now:            now,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = wallets.Insert(ctx, db.Pool, second.Wallet)
	if !errors.Is(err, database.ErrConflict) {
		t.Fatalf("err=%v want conflict", err)
	}
}

func TestGetByIDForUpdateAndUpdate(t *testing.T) {
	db := database.OpenTestDB(t)
	ctx := context.Background()
	wallets := database.NewWalletRepo()
	ledgers := database.NewLedgerRepo()
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

	opened, err := wallet.Open(wallet.OpenParams{
		WalletID:       "wallet-2",
		PlayerID:       "player-2",
		InitialBalance: mustParse(t, "100.00", "BRL"),
		OpeningTxID:    "tx-open-2",
		LedgerEntryID:  "ledger-open-2",
		Now:            now,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.WithinTx(ctx, func(q gateway.Querier) error {
		if err := wallets.Insert(ctx, q, opened.Wallet); err != nil {
			return err
		}
		return ledgers.Insert(ctx, q, *opened.Ledger)
	})
	if err != nil {
		t.Fatal(err)
	}

	err = db.WithinTx(ctx, func(q gateway.Querier) error {
		locked, err := wallets.GetByIDForUpdate(ctx, q, "wallet-2")
		if err != nil {
			return err
		}
		moved, err := locked.Debit("ledger-bet", "tx-bet", mustParse(t, "30.00", "BRL"), now.Add(time.Minute))
		if err != nil {
			return err
		}
		if err := wallets.Update(ctx, q, moved.Wallet); err != nil {
			return err
		}
		return ledgers.Insert(ctx, q, moved.Ledger)
	})
	if err != nil {
		t.Fatalf("debit tx: %v", err)
	}

	got, err := wallets.GetByID(ctx, db.Pool, "wallet-2")
	if err != nil {
		t.Fatal(err)
	}
	if got.Balance().AmountString() != "70.00" || got.Version() != 2 {
		t.Fatalf("balance=%s version=%d", got.Balance().AmountString(), got.Version())
	}
	entries, err := ledgers.ListByWalletID(ctx, db.Pool, "wallet-2")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries=%d", len(entries))
	}
}

func TestLedgerEntriesRejectUpdateDelete(t *testing.T) {
	db := database.OpenTestDB(t)
	ctx := context.Background()
	wallets := database.NewWalletRepo()
	ledgers := database.NewLedgerRepo()
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)

	opened, err := wallet.Open(wallet.OpenParams{
		WalletID: "wallet-imm", PlayerID: "player-imm",
		InitialBalance: mustParse(t, "10.00", "BRL"),
		OpeningTxID:    "tx-imm", LedgerEntryID: "ledger-imm", Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.WithinTx(ctx, func(q gateway.Querier) error {
		if err := wallets.Insert(ctx, q, opened.Wallet); err != nil {
			return err
		}
		return ledgers.Insert(ctx, q, *opened.Ledger)
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Pool.Exec(ctx, `UPDATE wallet_ledger_entries SET amount_minor = 1 WHERE id = $1`, "ledger-imm")
	if err == nil {
		t.Fatal("expected UPDATE to be rejected")
	}
	_, err = db.Pool.Exec(ctx, `DELETE FROM wallet_ledger_entries WHERE id = $1`, "ledger-imm")
	if err == nil {
		t.Fatal("expected DELETE to be rejected")
	}
}

func mustParse(t *testing.T, amount, currency string) money.Money {
	t.Helper()
	m, err := money.Parse(currency, amount)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
