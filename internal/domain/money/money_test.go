package money_test

import (
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/domain/money"
)

func TestParseOK(t *testing.T) {
	t.Parallel() //permite rodar os testes em paralelo, evitando dependências entre eles

	cases := []struct {
		amount   string
		currency string
		minor    int64
	}{
		{"0.00", "BRL", 0},
		{"25.00", "BRL", 2500},
		{"1000.00", "USD", 100000},
		{"0.01", "BRL", 1},
		{"0.99", "BRL", 99},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.amount+"_"+tc.currency, func(t *testing.T) {
			t.Parallel() //permite rodar os testes em paralelo
			m, err := money.Parse(tc.amount, tc.currency)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if m.Minor() != tc.minor {
				t.Fatalf("minor=%d want %d", m.Minor(), tc.minor)
			}
			if m.Currency() != tc.currency {
				t.Fatalf("currency=%q want %q", m.Currency(), tc.currency)
			}
			if m.AmountString() != tc.amount {
				t.Fatalf("AmountString=%q want %q", m.AmountString(), tc.amount)
			}
		})
	}
}

func TestParseInvalid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		amount   string
		currency string
		want     error
	}{
		{"empty_amount", "", "BRL", money.ErrInvalidAmount},
		{"nan", "NaN", "BRL", money.ErrInvalidAmount},
		{"infinity", "Infinity", "BRL", money.ErrInvalidAmount},
		{"scientific", "1e2", "BRL", money.ErrInvalidAmount},
		{"too_many_decimals", "25.001", "BRL", money.ErrInvalidAmount},
		{"no_decimals", "25", "BRL", money.ErrInvalidAmount},
		{"one_decimal", "25.0", "BRL", money.ErrInvalidAmount},
		{"negative", "-1.00", "BRL", money.ErrInvalidAmount},
		{"trailing_dot", "25.", "BRL", money.ErrInvalidAmount},
		{"plus_sign", "+25.00", "BRL", money.ErrInvalidAmount},
		{"empty_currency", "25.00", "", money.ErrInvalidCurrency},
		{"lowercase_currency", "25.00", "brl", money.ErrInvalidCurrency},
		{"long_currency", "25.00", "BRLX", money.ErrInvalidCurrency},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := money.Parse(tc.amount, tc.currency)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want %v", err, tc.want)
			}
		})
	}
}

func TestArithmetic(t *testing.T) {
	t.Parallel()

	a, err := money.Parse("25.00", "BRL")
	if err != nil {
		t.Fatal(err)
	}
	b, err := money.Parse("10.50", "BRL")
	if err != nil {
		t.Fatal(err)
	}

	sum, err := a.Add(b)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if sum.AmountString() != "35.50" {
		t.Fatalf("sum=%q", sum.AmountString())
	}

	diff, err := a.Sub(mustParse(t, "10.00", "BRL"))
	if err != nil {
		t.Fatalf("Sub: %v", err)
	}
	if diff.AmountString() != "15.00" {
		t.Fatalf("diff=%q", diff.AmountString())
	}

	neg, err := a.Neg()
	if err != nil {
		t.Fatalf("Neg: %v", err)
	}
	if neg.AmountString() != "-25.00" {
		t.Fatalf("neg=%q", neg.AmountString())
	}
	if !neg.IsNegative() {
		t.Fatal("expected negative")
	}
}

func TestCurrencyMismatch(t *testing.T) {
	t.Parallel()

	brl := mustParse(t, "10.00", "BRL")
	usd := mustParse(t, "10.00", "USD")

	if _, err := brl.Add(usd); !errors.Is(err, money.ErrCurrencyMismatch) {
		t.Fatalf("Add err=%v", err)
	}
	if _, err := brl.Sub(usd); !errors.Is(err, money.ErrCurrencyMismatch) {
		t.Fatalf("Sub err=%v", err)
	}
	if _, err := brl.Cmp(usd); !errors.Is(err, money.ErrCurrencyMismatch) {
		t.Fatalf("Cmp err=%v", err)
	}
}

func TestOverflow(t *testing.T) {
	t.Parallel()

	max, err := money.FromMinor(math.MaxInt64, "BRL")
	if err != nil {
		t.Fatal(err)
	}
	one, err := money.FromMinor(1, "BRL")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := max.Add(one); !errors.Is(err, money.ErrOverflow) {
		t.Fatalf("Add overflow err=%v", err)
	}

	min, err := money.FromMinor(math.MinInt64, "BRL")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := min.Neg(); !errors.Is(err, money.ErrOverflow) {
		t.Fatalf("Neg overflow err=%v", err)
	}
	if _, err := min.Sub(one); !errors.Is(err, money.ErrOverflow) {
		t.Fatalf("Sub overflow err=%v", err)
	}

	// Parse overflow: whole part too large for *100
	_, err = money.Parse("92233720368547759.00", "BRL")
	if !errors.Is(err, money.ErrOverflow) {
		t.Fatalf("Parse overflow err=%v", err)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	t.Parallel()

	m := mustParse(t, "25.00", "BRL")
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"amount":"25.00","currency":"BRL"}` {
		t.Fatalf("json=%s", raw)
	}

	var back money.Money
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if !m.Equal(back) {
		t.Fatalf("round-trip mismatch: %#v vs %#v", m, back)
	}
}

func TestZeroEqualCmp(t *testing.T) {
	t.Parallel()

	z, err := money.Zero("BRL")
	if err != nil {
		t.Fatal(err)
	}
	if !z.IsZero() {
		t.Fatal("expected zero")
	}
	parsed := mustParse(t, "0.00", "BRL")
	if !z.Equal(parsed) {
		t.Fatal("Zero should equal 0.00")
	}
	cmp, err := z.Cmp(parsed)
	if err != nil || cmp != 0 {
		t.Fatalf("Cmp=%d err=%v", cmp, err)
	}
}

func TestUninitialized(t *testing.T) {
	t.Parallel()

	var m money.Money
	if _, err := m.Add(m); !errors.Is(err, money.ErrUninitialized) {
		t.Fatalf("Add err=%v", err)
	}
	if _, err := m.Neg(); !errors.Is(err, money.ErrUninitialized) {
		t.Fatalf("Neg err=%v", err)
	}
	if _, err := m.MarshalJSON(); !errors.Is(err, money.ErrUninitialized) {
		t.Fatalf("MarshalJSON err=%v", err)
	}
	if m.Equal(m) {
		t.Fatal("uninitialized should not Equal itself")
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
