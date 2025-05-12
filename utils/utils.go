// Package utils provides utility functions for the application.
package utils

import (
	"math/rand"

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
	return int64(rand.Intn(9000000000) + 1000000000)
}
