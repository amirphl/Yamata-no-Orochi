// Package models contains domain entities and business models for the authentication system
package models

import (
	"time"

	"github.com/google/uuid"
)

type OTPVerification struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	CorrelationID uuid.UUID  `gorm:"type:uuid;not null;index:idx_otp_correlation_id" json:"correlation_id"` // Groups related OTP records
	CustomerID    uint       `gorm:"not null;index:idx_otp_customer_id" json:"customer_id"`
	Customer      Customer   `gorm:"foreignKey:CustomerID;references:ID" json:"customer,omitempty"`
	OTPCode       string     `gorm:"size:6;not null" json:"-"` // Never serialize OTP code
	OTPType       string     `gorm:"type:otp_type_enum;not null;index:idx_otp_type_status" json:"otp_type"`
	TargetValue   string     `gorm:"size:255;not null" json:"target_value"`
	Status        string     `gorm:"type:otp_status_enum;default:pending;index:idx_otp_type_status" json:"status"`
	AttemptsCount int        `gorm:"default:0" json:"attempts_count"`
	MaxAttempts   int        `gorm:"default:3" json:"max_attempts"`
	CreatedAt     time.Time  `gorm:"default:CURRENT_TIMESTAMP;index:idx_otp_created_at" json:"created_at"`
	ExpiresAt     time.Time  `gorm:"not null;index:idx_otp_expires_at" json:"expires_at"`
	VerifiedAt    *time.Time `json:"verified_at,omitempty"`
	IPAddress     *string    `gorm:"type:inet;index:idx_otp_ip_address" json:"ip_address,omitempty"`
	UserAgent     *string    `gorm:"type:text" json:"user_agent,omitempty"`
}

func (OTPVerification) TableName() string {
	return "otp_verifications"
}

// OTP type constants
const (
	OTPTypeMobile        = "mobile"
	OTPTypeEmail         = "email"
	OTPTypePasswordReset = "password_reset"
)

// OTP status constants
const (
	OTPStatusPending  = "pending"
	OTPStatusVerified = "verified"
	OTPStatusExpired  = "expired"
	OTPStatusFailed   = "failed"
	OTPStatusUsed     = "used"
)

// OTPVerificationFilter represents filter criteria for OTP verification queries
type OTPVerificationFilter struct {
	ID            *uint
	CustomerID    *uint
	OTPType       *string
	TargetValue   *string
	Status        *string
	IPAddress     *string
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	ExpiresAfter  *time.Time
	ExpiresBefore *time.Time
	IsActive      *bool // Helper to filter non-expired pending OTPs
}

func (o *OTPVerification) IsExpired() bool {
	return time.Now().After(o.ExpiresAt)
}

func (o *OTPVerification) IsVerified() bool {
	return o.Status == OTPStatusVerified
}

func (o *OTPVerification) IsPending() bool {
	return o.Status == OTPStatusPending
}

func (o *OTPVerification) CanAttempt() bool {
	return o.AttemptsCount < o.MaxAttempts && !o.IsExpired() && o.IsPending()
}
