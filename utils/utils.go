// Package utils provides utility functions for the application.
package utils

import (
	"crypto/rand"

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

// GenerateRandomAgencyRefererCode generates a random 10-digit string for agency_referer_code
func GenerateRandomAgencyRefererCode() string {
	// Ensure first digit is non-zero to keep 10 digits
	digits := make([]byte, 10)
	for i := 0; i < 10; i++ {
		var b [1]byte
		for {
			_, _ = rand.Read(b[:])
			v := b[0] % 10
			if i == 0 && v == 0 {
				continue
			}
			digits[i] = '0' + v
			break
		}
	}
	return string(digits)
}
