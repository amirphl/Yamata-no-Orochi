// Package utils provides utility functions for the application.
package utils

import (
	"crypto/rand"
	"math/big"

	"github.com/google/uuid"
)

func ToPtr[T any](v T) *T {
	return &v
}

func IsTrue(b *bool) bool {
	return b != nil && *b
}

// ParseUUID parses a UUID string and returns a uuid.UUID
func ParseUUID(uuidStr string) (uuid.UUID, error) {
	return uuid.Parse(uuidStr)
}

// GenerateRandomAgencyRefererCode generates a random 10-digit integer for agency_referer_code
func GenerateRandomAgencyRefererCode() int64 {
	// Generate random number between 1000000000 and 9999999999
	max := big.NewInt(9999999999)
	min := big.NewInt(1000000000)

	n, err := rand.Int(rand.Reader, new(big.Int).Sub(max, min))
	if err != nil {
		// Fallback to a simple random number if crypto/rand fails
		return 1000000000 + UTCNow().UnixNano()%9000000000
	}

	return new(big.Int).Add(n, min).Int64()
}
