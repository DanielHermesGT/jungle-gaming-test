package wager

// Origin distinguishes internal OPENING from provider-facing operations.
type Origin string

const (
	OriginInternal Origin = "INTERNAL"
	OriginExternal Origin = "EXTERNAL"
)

// ParseOrigin validates an origin string.
func ParseOrigin(s string) (Origin, error) {
	o := Origin(s)
	switch o {
	case OriginInternal, OriginExternal:
		return o, nil
	default:
		return "", ErrInvalidOrigin
	}
}
