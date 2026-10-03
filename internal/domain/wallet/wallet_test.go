package wallet_test

import (
	"errors"
	"testing"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/wallet"
)

func TestOpenPositiveBalance(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	bal := mustParse(t, "1000.00", "BRL")

	res, err := wallet.Open(wallet.OpenParams{
		WalletID:       "wallet-1",
		PlayerID:       "player-1",
		InitialBalance: bal,
		OpeningTxID:    "tx-opening",
		LedgerEntryID:  "ledger-1",
		Now:            now,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if res.Wallet.Version() != 1 {
		t.Fatalf("version=%d", res.Wallet.Version())
	}
	if !res.Wallet.Balance().Equal(bal) {
		t.Fatalf("balance=%s", res.Wallet.Balance().AmountString())
	}
	if res.Ledger == nil {
		t.Fatal("expected ledger entry")
	}
	if res.Ledger.Direction() != wallet.DirectionCredit {
		t.Fatalf("direction=%s", res.Ledger.Direction())
	}
	if res.Ledger.BalanceBefore().AmountString() != "0.00" {
		t.Fatalf("before=%s", res.Ledger.BalanceBefore().AmountString())
	}
	if res.Ledger.BalanceAfter().AmountString() != "1000.00" {
		t.Fatalf("after=%s", res.Ledger.BalanceAfter().AmountString())
	}
	if res.Ledger.TransactionID() != "tx-opening" {
		t.Fatalf("tx=%s", res.Ledger.TransactionID())
	}
}

func TestOpenZeroBalanceNoLedger(t *testing.T) {
	t.Parallel()

	bal := mustParse(t, "0.00", "BRL")
	res, err := wallet.Open(wallet.OpenParams{
		WalletID:       "wallet-1",
		PlayerID:       "player-1",
		InitialBalance: bal,
		Now:            time.Unix(1, 0).UTC(),
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if res.Wallet.Version() != 1 {
		t.Fatalf("version=%d", res.Wallet.Version())
	}
	if res.Ledger != nil {
		t.Fatal("expected no ledger for zero balance")
	}
}

func TestOpenInvalid(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0).UTC()
	bal := mustParse(t, "10.00", "BRL")

	cases := []struct {
		name string
		p    wallet.OpenParams
		want error
	}{
		{
			name: "empty_wallet",
			p:    wallet.OpenParams{PlayerID: "p", InitialBalance: bal, OpeningTxID: "t", LedgerEntryID: "l", Now: now},
			want: wallet.ErrInvalidWallet,
		},
		{
			name: "empty_player",
			p:    wallet.OpenParams{WalletID: "w", InitialBalance: bal, OpeningTxID: "t", LedgerEntryID: "l", Now: now},
			want: wallet.ErrInvalidPlayer,
		},
		{
			name: "uninitialized_money",
			p:    wallet.OpenParams{WalletID: "w", PlayerID: "p", Now: now},
			want: wallet.ErrInvalidMovement,
		},
		{
			name: "positive_without_ids",
			p:    wallet.OpenParams{WalletID: "w", PlayerID: "p", InitialBalance: bal, Now: now},
			want: wallet.ErrInvalidMovement,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := wallet.Open(tc.p)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want %v", err, tc.want)
			}
		})
	}
}

func TestCreditAndDebit(t *testing.T) {
	t.Parallel()

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

	credited, err := opened.Wallet.Credit("ledger-c", "tx-win", mustParse(t, "20.00", "BRL"), now.Add(time.Second))
	if err != nil {
		t.Fatalf("Credit: %v", err)
	}
	if credited.Wallet.Version() != 2 {
		t.Fatalf("version=%d want 2", credited.Wallet.Version())
	}
	if credited.Wallet.Balance().AmountString() != "120.00" {
		t.Fatalf("balance=%s", credited.Wallet.Balance().AmountString())
	}
	if credited.Ledger.Direction() != wallet.DirectionCredit {
		t.Fatal("expected CREDIT")
	}
	if credited.Ledger.BalanceBefore().AmountString() != "100.00" || credited.Ledger.BalanceAfter().AmountString() != "120.00" {
		t.Fatalf("ledger before/after=%s/%s", credited.Ledger.BalanceBefore().AmountString(), credited.Ledger.BalanceAfter().AmountString())
	}

	debited, err := credited.Wallet.Debit("ledger-d", "tx-bet", mustParse(t, "30.00", "BRL"), now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("Debit: %v", err)
	}
	if debited.Wallet.Version() != 3 {
		t.Fatalf("version=%d want 3", debited.Wallet.Version())
	}
	if debited.Wallet.Balance().AmountString() != "90.00" {
		t.Fatalf("balance=%s", debited.Wallet.Balance().AmountString())
	}
	if debited.Ledger.Direction() != wallet.DirectionDebit {
		t.Fatal("expected DEBIT")
	}
	if debited.Ledger.BalanceBefore().AmountString() != "120.00" || debited.Ledger.BalanceAfter().AmountString() != "90.00" {
		t.Fatalf("ledger before/after=%s/%s", debited.Ledger.BalanceBefore().AmountString(), debited.Ledger.BalanceAfter().AmountString())
	}
}

func TestDebitInsufficientFunds(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0).UTC()
	opened, err := wallet.Open(wallet.OpenParams{
		WalletID:       "wallet-1",
		PlayerID:       "player-1",
		InitialBalance: mustParse(t, "50.00", "BRL"),
		OpeningTxID:    "tx-open",
		LedgerEntryID:  "ledger-open",
		Now:            now,
	})
	if err != nil {
		t.Fatal(err)
	}

	before := opened.Wallet
	_, err = opened.Wallet.Debit("ledger-d", "tx-bet", mustParse(t, "80.00", "BRL"), now)
	if !errors.Is(err, wallet.ErrInsufficientFunds) {
		t.Fatalf("err=%v", err)
	}
	if before.Version() != opened.Wallet.Version() || !before.Balance().Equal(opened.Wallet.Balance()) {
		t.Fatal("wallet must remain unchanged after failed debit")
	}
}

func TestCurrencyMismatchAndInvalidMovement(t *testing.T) {
	t.Parallel()

	now := time.Unix(1, 0).UTC()
	opened, err := wallet.Open(wallet.OpenParams{
		WalletID:       "wallet-1",
		PlayerID:       "player-1",
		InitialBalance: mustParse(t, "50.00", "BRL"),
		OpeningTxID:    "tx-open",
		LedgerEntryID:  "ledger-open",
		Now:            now,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := opened.Wallet.Credit("l", "t", mustParse(t, "1.00", "USD"), now); !errors.Is(err, money.ErrCurrencyMismatch) {
		t.Fatalf("currency err=%v", err)
	}
	if _, err := opened.Wallet.Credit("l", "t", mustParse(t, "0.00", "BRL"), now); !errors.Is(err, wallet.ErrInvalidMovement) {
		t.Fatalf("zero err=%v", err)
	}
	neg, err := money.FromMinor(-100, "BRL")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := opened.Wallet.Debit("l", "t", neg, now); !errors.Is(err, wallet.ErrInvalidMovement) {
		t.Fatalf("negative err=%v", err)
	}
}

func TestRehydrateThenDebit(t *testing.T) {
	t.Parallel()

	now := time.Unix(10, 0).UTC()
	bal := mustParse(t, "100.00", "BRL")
	w, err := wallet.Rehydrate("wallet-1", "player-1", bal, 1, now, now)
	if err != nil {
		t.Fatal(err)
	}
	if w.Version() != 1 || !w.Balance().Equal(bal) {
		t.Fatal("rehydrate must preserve state")
	}

	res, err := w.Debit("ledger-1", "tx-1", mustParse(t, "40.00", "BRL"), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if res.Wallet.Balance().AmountString() != "60.00" || res.Wallet.Version() != 2 {
		t.Fatalf("balance=%s version=%d", res.Wallet.Balance().AmountString(), res.Wallet.Version())
	}
}

func TestUninitializedWallet(t *testing.T) {
	t.Parallel()

	var w wallet.Wallet
	_, err := w.Credit("l", "t", mustParse(t, "1.00", "BRL"), time.Unix(1, 0).UTC())
	if !errors.Is(err, wallet.ErrUninitialized) {
		t.Fatalf("err=%v", err)
	}
}

func mustParse(t *testing.T, amount, currency string) money.Money {
	t.Helper()
	m, err := money.Parse(amount, currency)
	if err != nil {
		t.Fatalf("Parse(%q,%q): %v", amount, currency, err)
	}
	return m
}
