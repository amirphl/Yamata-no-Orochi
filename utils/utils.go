// Package utils provides utility functions for the application.
package utils

import (
	"crypto/rand"
	"errors"
	"strings"
	"unicode"

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
	for i := range 10 {
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

func ValidateShebaNumber(shebaNumber *string) (string, error) {
	if shebaNumber == nil || len(*shebaNumber) == 0 {
		return "", errors.New("sheba number is required")
	}
	shebaNumberStr := strings.TrimSpace(*shebaNumber)
	// validate prefix IR
	if !strings.HasPrefix(shebaNumberStr, "IR") {
		return "", errors.New("sheba number must start with IR")
	}
	// validate length exactly 26
	if len(shebaNumberStr) != 26 {
		return "", errors.New("sheba number must be 26 digits")
	}
	// validate digits are numbers
	for _, s := range shebaNumberStr[2:] {
		if !unicode.IsDigit(s) {
			return "", errors.New("sheba number must contain only digits")
		}
	}

	return shebaNumberStr, nil
}
