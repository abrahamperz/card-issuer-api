package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// ErrInvalidHMACKeySize is returned when the HMAC key size is too small.
var ErrInvalidHMACKeySize = errors.New("crypto: invalid HMAC key size, expected at least 32 bytes")

// BlindIndexer defines the interface for creating deterministic, non-reversible indexes.
type BlindIndexer interface {
	ComputeIndex(value []byte) string
}

type hmacBlindIndexer struct {
	key []byte
}

// NewHMACBlindIndexer creates a new BlindIndexer using HMAC-SHA256.
func NewHMACBlindIndexer(key []byte) (BlindIndexer, error) {
	if len(key) < 32 {
		return nil, ErrInvalidHMACKeySize
	}

	// Copy the key to prevent external modification
	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)

	return &hmacBlindIndexer{
		key: keyCopy,
	}, nil
}

// ComputeIndex computes the HMAC-SHA256 of the value and returns it as a hex string.
func (h *hmacBlindIndexer) ComputeIndex(value []byte) string {
	mac := hmac.New(sha256.New, h.key)
	mac.Write(value)
	digest := mac.Sum(nil)
	return hex.EncodeToString(digest)
}
