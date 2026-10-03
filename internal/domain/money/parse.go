package money

import (
	"math"
	"strconv"
	"strings"
)

// Parse builds Money from currency and an external decimal amount string.
// Amount must match digits + '.' + exactly two fractional digits (e.g. "25.00").
// Negative amounts, scientific notation, and silent rounding are rejected.
func Parse(currency, amount string) (Money, error) {
	if err := validateCurrency(currency); err != nil {
		return Money{}, err
	}
	if amount == "" {
		return Money{}, ErrInvalidAmount
	}
	lower := strings.ToLower(amount)
	switch lower {
	case "nan", "inf", "+inf", "-inf", "infinity", "+infinity", "-infinity":
		return Money{}, ErrInvalidAmount
	}
	if strings.ContainsAny(amount, "eE+") {
		return Money{}, ErrInvalidAmount
	}
	if amount[0] == '-' {
		return Money{}, ErrInvalidAmount
	}

	dot := strings.IndexByte(amount, '.')
	if dot <= 0 || dot != len(amount)-3 {
		return Money{}, ErrInvalidAmount
	}
	wholePart := amount[:dot]
	fracPart := amount[dot+1:]
	if len(fracPart) != scale {
		return Money{}, ErrInvalidAmount
	}
	if !allDigits(wholePart) || !allDigits(fracPart) {
		return Money{}, ErrInvalidAmount
	}

	frac, err := strconv.ParseInt(fracPart, 10, 64)
	if err != nil {
		return Money{}, ErrInvalidAmount
	}

	// Avoid float; combine whole*100 + frac with overflow checks.
	if wholePart == "0" {
		return Money{minor: frac, currency: currency}, nil
	}

	whole, err := strconv.ParseInt(wholePart, 10, 64)
	if err != nil {
		return Money{}, ErrInvalidAmount
	}
	if whole > math.MaxInt64/100 {
		return Money{}, ErrOverflow
	}
	minor := whole * 100
	sum, err := addMinor(minor, frac)
	if err != nil {
		return Money{}, err
	}
	return Money{minor: sum, currency: currency}, nil
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
