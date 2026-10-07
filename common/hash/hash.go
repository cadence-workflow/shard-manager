package hash

import (
	"encoding/binary"

	farm "github.com/dgryski/go-farm"
)

// FingerprintSize is the length in bytes of a Fingerprint64 result.
const FingerprintSize = 8

// Fingerprint64 returns a deterministic 64-bit FarmHash fingerprint encoded as
// big-endian bytes.
func Fingerprint64(value []byte) []byte {
	key := make([]byte, FingerprintSize)
	binary.BigEndian.PutUint64(key, farm.Fingerprint64(value))
	return key
}

// Fingerprint64String returns the fingerprint of value.
func Fingerprint64String(value string) []byte {
	return Fingerprint64([]byte(value))
}
