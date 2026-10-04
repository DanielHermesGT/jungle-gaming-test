package wallet_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/database"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wager"
	domainwallet "github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wallet"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/gateway"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/usecase"
	usecasewallet "github.com/DanielHermesGT/jungle-gaming-test/internal/usecase/wallet"
)

func TestOpenPositiveAndZero(t *testing.T) {
	uc, db := newUseCase(t)
	ctx := context.Background()

	pos, err := uc.Open(ctx, usecasewallet.OpenInput{
		PlayerID:       "player-1",
		InitialBalance: mustParse(t, "100.00", "BRL"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if pos.Balance.AmountString() != "100.00" || pos.Version != 1 {
		t.Fatalf("got=%+v", pos)
	}

	got, err := uc.Get(ctx, pos.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != pos.ID {
		t.Fatalf("get=%+v", got)
	}

	page, err := uc.ListLedger(ctx, usecasewallet.ListLedgerInput{WalletID: pos.ID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 1 || page.Entries[0].Direction != domainwallet.DirectionCredit {
		t.Fatalf("ledger=%+v", page.Entries)
	}

	opening, err := database.NewWagerRepo().GetByID(ctx, db.Pool, page.Entries[0].TransactionID)
	if err != nil {
		t.Fatalf("opening wager: %v", err)
	}
	if opening.Kind() != wager.KindOpening || opening.Status() != wager.StatusProcessed {
		t.Fatalf("opening=%s %s", opening.Kind(), opening.Status())
	}

	zero, err := uc.Open(ctx, usecasewallet.OpenInput{
		PlayerID:       "player-zero",
		InitialBalance: mustParse(t, "0.00", "BRL"),
	})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := uc.ListLedger(ctx, usecasewallet.ListLedgerInput{WalletID: zero.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Entries) != 0 {
		t.Fatalf("want 0 entries, got %d", len(empty.Entries))
	}
	var n int
	if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM wager_transactions WHERE wallet_id = $1`, zero.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("zero open must not create OPENING, got %d", n)
	}
}

func TestOpenConflict(t *testing.T) {
	uc, _ := newUseCase(t)
	ctx := context.Background()
	bal := mustParse(t, "0.00", "BRL")

	if _, err := uc.Open(ctx, usecasewallet.OpenInput{PlayerID: "dup", InitialBalance: bal}); err != nil {
		t.Fatal(err)
	}
	_, err := uc.Open(ctx, usecasewallet.OpenInput{PlayerID: "dup", InitialBalance: bal})
	if !errors.Is(err, usecase.ErrConflict) {
		t.Fatalf("err=%v", err)
	}
}

func TestGetNotFound(t *testing.T) {
	uc, _ := newUseCase(t)
	_, err := uc.Get(context.Background(), "missing")
	if !errors.Is(err, usecase.ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestListLedgerPagesAndReconcile(t *testing.T) {
	db := database.OpenTestDB(t)
	ctx := context.Background()
	ids := &seqIDs{}
	clk := fixedClock{at: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)}
	uc := usecasewallet.NewUseCase(db, database.NewWalletRepo(), database.NewLedgerRepo(), database.NewWagerRepo(), ids, clk)

	opened, err := uc.Open(ctx, usecasewallet.OpenInput{
		PlayerID:       "player-page",
		InitialBalance: mustParse(t, "100.00", "BRL"),
	})
	if err != nil {
		t.Fatal(err)
	}

	wallets := database.NewWalletRepo()
	ledgers := database.NewLedgerRepo()
	err = db.WithinTx(ctx, func(q gateway.Querier) error {
		w, err := wallets.GetByIDForUpdate(ctx, q, opened.ID)
		if err != nil {
			return err
		}
		m1, err := w.Debit(ids.New(), ids.New(), mustParse(t, "10.00", "BRL"), clk.Now())
		if err != nil {
			return err
		}
		if err := wallets.Update(ctx, q, m1.Wallet); err != nil {
			return err
		}
		if err := ledgers.Insert(ctx, q, m1.Ledger); err != nil {
			return err
		}
		m2, err := m1.Wallet.Debit(ids.New(), ids.New(), mustParse(t, "5.00", "BRL"), clk.Now())
		if err != nil {
			return err
		}
		if err := wallets.Update(ctx, q, m2.Wallet); err != nil {
			return err
		}
		return ledgers.Insert(ctx, q, m2.Ledger)
	})
	if err != nil {
		t.Fatal(err)
	}

	page1, err := uc.ListLedger(ctx, usecasewallet.ListLedgerInput{WalletID: opened.ID, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page1.Entries) != 2 || page1.NextCursor == "" {
		t.Fatalf("page1=%+v", page1)
	}
	page2, err := uc.ListLedger(ctx, usecasewallet.ListLedgerInput{
		WalletID: opened.ID,
		Cursor:   page1.NextCursor,
		Limit:    2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Entries) != 1 || page2.NextCursor != "" {
		t.Fatalf("page2=%+v", page2)
	}

	ok, err := uc.Reconcile(ctx, opened.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok.Consistent || ok.CheckedEntries != 3 {
		t.Fatalf("reconcile=%+v", ok)
	}

	_, err = db.Pool.Exec(ctx, `UPDATE wallets SET balance_minor = balance_minor + 1 WHERE id = $1`, opened.ID)
	if err != nil {
		t.Fatal(err)
	}
	bad, err := uc.Reconcile(ctx, opened.ID)
	if err != nil {
		t.Fatal(err)
	}
	if bad.Consistent || bad.Difference.AmountString() != "0.01" {
		t.Fatalf("want inconsistent, got %+v", bad)
	}
}

func newUseCase(t *testing.T) (*usecasewallet.UseCase, *database.DB) {
	t.Helper()
	db := database.OpenTestDB(t)
	uc := usecasewallet.NewUseCase(
		db,
		database.NewWalletRepo(),
		database.NewLedgerRepo(),
		database.NewWagerRepo(),
		&seqIDs{},
		fixedClock{at: time.Date(2026, 10, 3, 15, 0, 0, 0, time.UTC)},
	)
	return uc, db
}

type seqIDs struct{ n int }

func (s *seqIDs) New() string {
	s.n++
	return fmt.Sprintf("id-%d", s.n)
}

type fixedClock struct{ at time.Time }

func (c fixedClock) Now() time.Time { return c.at }

func mustParse(t *testing.T, amount, currency string) money.Money {
	t.Helper()
	m, err := money.Parse(currency, amount)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
