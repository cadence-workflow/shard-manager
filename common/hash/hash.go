package hash

import (
	"encoding/binary"

	farm "github.com/dgryski/go-farm"
)

// fingerprintSize is the length in bytes of a Fingerprint64 result.
const fingerprintSize = 8

// Fingerprint64 returns a deterministic 64-bit FarmHash fingerprint encoded as big-endian bytes.
func Fingerprint64(value []byte) []byte {
	key := make([]byte, fingerprintSize)
	binary.BigEndian.PutUint64(key, farm.Fingerprint64(value))
	return key
}

// Fingerprint64String is Fingerprint64 for string keys.
func Fingerprint64String(value string) []byte {
	return Fingerprint64([]byte(value))
}

// IsMinFingerprint reports whether key is the minimum Fingerprint64 value
// (an all-zero fingerprint of the expected width).
func IsMinFingerprint(key []byte) bool {
	if len(key) != fingerprintSize {
		return false
	}
	for _, b := range key {
		if b != 0 {
			return false
		}
	}
	return true
}
