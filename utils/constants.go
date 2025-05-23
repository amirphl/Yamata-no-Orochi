package utils

import "time"

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
