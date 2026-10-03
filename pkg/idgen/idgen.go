package idgen

import (
	"crypto/rand"
	"fmt"
)

// Generator produces opaque string IDs.
type Generator interface {
	New() string
}

// UUID generates RFC 4122 version-4 IDs.
type UUID struct{}

func (UUID) New() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
