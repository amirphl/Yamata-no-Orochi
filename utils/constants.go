package utils

import (
	"time"
)

type ContextKey string

const (
	RequestIDKey  ContextKey = "X-Request-ID"
	UserAgentKey  ContextKey = "User-Agent"
	IPAddressKey  ContextKey = "IP-Address"
	EndpointKey   ContextKey = "Endpoint"
	TimeoutKey    ContextKey = "Timeout"
	CancelFuncKey ContextKey = "Cancel-Func"
	CustomerIDKey ContextKey = "Customer-ID"
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
)

// CORS and security constants
const (
	// CORSMaxAge is the maximum age for CORS preflight requests (24 hours)
	CORSMaxAge = 86400
)

// Tax and payment constants
const (
	MinAcceptableCampaignCapacity = 500

	TomanCurrency = "TMN"
)

const (
	DefaultReferrerAgencyCode = "jazebeh.ir"
)

const (
	AudienceSpecCacheKey = "audience_spec:cache"
	AudienceSpecLockKey  = "audience_spec:lock"
)
