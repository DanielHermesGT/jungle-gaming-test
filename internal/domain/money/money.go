package money

import (
	"encoding/json"
	"fmt"
	"math"
)

const scale = 2

// Money is an immutable monetary value in minor units (fixed scale of 2).
type Money struct {
	minor    int64
	currency string
}

type moneyJSON struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

func (m Money) MarshalJSON() ([]byte, error) {
	if !m.valid() {
		return nil, ErrUninitialized
	}
	return json.Marshal(moneyJSON{
		Amount:   m.AmountString(),
		Currency: m.currency,
	})
}

func (m *Money) UnmarshalJSON(b []byte) error {
	var raw moneyJSON
	if err := json.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidAmount, err)
	}
	parsed, err := Parse(raw.Amount, raw.Currency)
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}

// FromMinor cria Money a partir de unidades mínimas (centavos).
// Valores negativos são permitidos para uso interno (diferenças e cálculos).
func FromMinor(minor int64, currency string) (Money, error) {
	if err := validateCurrency(currency); err != nil {
		return Money{}, err
	}
	return Money{minor: minor, currency: currency}, nil
}

// Zero returns a zero amount for the given currency.
func Zero(currency string) (Money, error) {
	return FromMinor(0, currency)
}

func (m Money) valid() bool {
	return m.currency != ""
}

func (m Money) Minor() int64 {
	return m.minor
}

func (m Money) Currency() string {
	return m.currency
}

// AmountString formata o valor com exatamente duas casas decimais (ex.: "25.00").
func (m Money) AmountString() string {
	if !m.valid() {
		return ""
	}
	sign := ""
	minor := m.minor
	if minor < 0 {
		sign = "-"
		minor = -minor
	}
	whole := minor / 100
	frac := minor % 100
	return fmt.Sprintf("%s%d.%02d", sign, whole, frac)
}

func (m Money) IsZero() bool {
	return m.valid() && m.minor == 0
}

func (m Money) IsNegative() bool {
	return m.valid() && m.minor < 0
}

func (m Money) Equal(other Money) bool {
	return m.valid() && other.valid() && m.currency == other.currency && m.minor == other.minor
}

// Cmp compara dois Money da mesma moeda.
// Retorna -1 se m < other, 0 se iguais, 1 se m > other.
func (m Money) Cmp(other Money) (int, error) {
	if !m.valid() || !other.valid() {
		return 0, ErrUninitialized
	}
	if m.currency != other.currency {
		return 0, ErrCurrencyMismatch
	}
	switch {
	case m.minor < other.minor:
		return -1, nil
	case m.minor > other.minor:
		return 1, nil
	default:
		return 0, nil
	}
}

func (m Money) Add(other Money) (Money, error) {
	if !m.valid() || !other.valid() {
		return Money{}, ErrUninitialized
	}
	if m.currency != other.currency {
		return Money{}, ErrCurrencyMismatch
	}
	sum, err := addMinor(m.minor, other.minor)
	if err != nil {
		return Money{}, err
	}
	return Money{minor: sum, currency: m.currency}, nil
}

func (m Money) Sub(other Money) (Money, error) {
	if !m.valid() || !other.valid() {
		return Money{}, ErrUninitialized
	}
	if m.currency != other.currency {
		return Money{}, ErrCurrencyMismatch
	}
	neg, err := negMinor(other.minor)
	if err != nil {
		return Money{}, err
	}
	sum, err := addMinor(m.minor, neg)
	if err != nil {
		return Money{}, err
	}
	return Money{minor: sum, currency: m.currency}, nil
}

func (m Money) Neg() (Money, error) {
	if !m.valid() {
		return Money{}, ErrUninitialized
	}
	neg, err := negMinor(m.minor)
	if err != nil {
		return Money{}, err
	}
	return Money{minor: neg, currency: m.currency}, nil
}

func addMinor(a, b int64) (int64, error) {
	if b > 0 && a > math.MaxInt64-b {
		return 0, ErrOverflow
	}
	if b < 0 && a < math.MinInt64-b {
		return 0, ErrOverflow
	}
	return a + b, nil
}

func negMinor(v int64) (int64, error) {
	if v == math.MinInt64 {
		return 0, ErrOverflow
	}
	return -v, nil
}

func validateCurrency(currency string) error {
	if len(currency) != 3 {
		return ErrInvalidCurrency
	}
	for i := 0; i < 3; i++ {
		c := currency[i]
		if c < 'A' || c > 'Z' {
			return ErrInvalidCurrency
		}
	}
	return nil
}
