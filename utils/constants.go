package utils

import (
	"time"

	"github.com/google/uuid"
)

// Token and session time constants
const (
	// AccessTokenTTL is the time-to-live for access tokens (24 hours)
	AccessTokenTTL = 24 * time.Hour

	// AccessTokenTTLSeconds is the time-to-live for access tokens in seconds (86400 seconds = 24 hours)
	AccessTokenTTLSeconds = 86400

	// RefreshTokenTTL is the time-to-live for refresh tokens (7 days)
	RefreshTokenTTL = 7 * 24 * time.Hour

	// SessionTimeout is the default session timeout (24 hours)
	SessionTimeout = 24 * time.Hour

	// OTPExpiry is the time-to-live for OTP codes (5 minutes)
	OTPExpiry = 5 * time.Minute

	// OTPExpirySeconds is the time-to-live for OTP codes in seconds (300 seconds = 5 minutes)
	OTPExpirySeconds = 300
)

// CORS and security constants
const (
	// CORSMaxAge is the maximum age for CORS preflight requests (24 hours)
	CORSMaxAge = 86400
)

// Tax and payment constants
const (
	// TaxRate is the tax rate applied to payments (10%)
	TaxRate = 0.10

	// TaxWalletUUID is the UUID of the system tax wallet
	TaxWalletUUID = "2672a1bf-b344-4d84-adee-5b92307a2e7c"
)

// GetTaxWalletUUID returns the tax wallet UUID as a uuid.UUID
func GetTaxWalletUUID() uuid.UUID {
	parsed, _ := uuid.Parse(TaxWalletUUID)
	return parsed
}
