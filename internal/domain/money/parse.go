package money

import (
	"math"
	"strconv"
	"strings"
)

// Parse builds Money from an external decimal amount string and currency.
// Amount must match digits + '.' + exactly two fractional digits (e.g. "25.00").
// Negative amounts, scientific notation, and silent rounding are rejected.
func Parse(amount, currency string) (Money, error) {
	if err := validateCurrency(currency); err != nil {
		return Money{}, err
	}
	minor, err := parseAmountMinor(amount)
	if err != nil {
		return Money{}, err
	}
	return Money{minor: minor, currency: currency}, nil
}

func parseAmountMinor(amount string) (int64, error) {
	if amount == "" {
		return 0, ErrInvalidAmount
	}
	lower := strings.ToLower(amount)
	switch lower {
	case "nan", "inf", "+inf", "-inf", "infinity", "+infinity", "-infinity":
		return 0, ErrInvalidAmount
	}
	if strings.ContainsAny(amount, "eE+") {
		return 0, ErrInvalidAmount
	}
	if amount[0] == '-' {
		return 0, ErrInvalidAmount
	}

	dot := strings.IndexByte(amount, '.')
	if dot <= 0 || dot != len(amount)-3 {
		return 0, ErrInvalidAmount
	}
	wholePart := amount[:dot]
	fracPart := amount[dot+1:]
	if len(fracPart) != scale {
		return 0, ErrInvalidAmount
	}
	if !allDigits(wholePart) || !allDigits(fracPart) {
		return 0, ErrInvalidAmount
	}

	frac, err := strconv.ParseInt(fracPart, 10, 64)
	if err != nil {
		return 0, ErrInvalidAmount
	}

	// Avoid float; combine whole*100 + frac with overflow checks.
	if wholePart == "0" {
		return frac, nil
	}

	whole, err := strconv.ParseInt(wholePart, 10, 64)
	if err != nil {
		return 0, ErrInvalidAmount
	}
	if whole > math.MaxInt64/100 {
		return 0, ErrOverflow
	}
	minor := whole * 100
	sum, err := addMinor(minor, frac)
	if err != nil {
		return 0, err
	}
	return sum, nil
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
