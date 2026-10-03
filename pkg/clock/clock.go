package clock

import "time"

// Clock provides the current time.
type Clock interface {
	Now() time.Time
}

// System returns UTC wall-clock time.
type System struct{}

func (System) Now() time.Time {
	return time.Now().UTC()
}
