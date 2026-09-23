package store

import (
	"crypto/rand"
	"encoding/hex"
)

// NewRunID returns a random, URL-safe run id.
func NewRunID() string {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
