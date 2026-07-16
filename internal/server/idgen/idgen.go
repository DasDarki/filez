// Package idgen produces short, URL-safe, unguessable file identifiers.
package idgen

import (
	"crypto/rand"
)

const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// DefaultLength is the number of characters in a generated id.
const DefaultLength = 10

// New returns a random base62 id of length n using a CSPRNG.
func New(n int) string {
	if n <= 0 {
		n = DefaultLength
	}
	buf := make([]byte, n)
	// rand.Read never returns an error for crypto/rand.
	_, _ = rand.Read(buf)
	for i, b := range buf {
		buf[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(buf)
}

// NewKey returns a longer token suitable for access keys.
func NewKey() string {
	return New(32)
}
