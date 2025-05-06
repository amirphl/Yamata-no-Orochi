// Package models contains domain entities and business models for the authentication system
package models

import (
	"encoding/json"
	"time"
)

type AuditLog struct {
	ID           uint            `gorm:"primaryKey" json:"id"`
	CustomerID   *uint           `gorm:"index:idx_audit_customer_id" json:"customer_id,omitempty"`
	Customer     *Customer       `gorm:"foreignKey:CustomerID;references:ID" json:"customer,omitempty"`
	Action       string          `gorm:"type:audit_action_enum;not null;index:idx_audit_action" json:"action"`
	Description  *string         `gorm:"type:text" json:"description,omitempty"`
	IPAddress    *string         `gorm:"type:inet;index:idx_audit_ip_address" json:"ip_address,omitempty"`
	UserAgent    *string         `gorm:"type:text" json:"user_agent,omitempty"`
	RequestID    *string         `gorm:"size:255;index:idx_audit_request_id" json:"request_id,omitempty"`
	Metadata     json.RawMessage `gorm:"type:jsonb;index:idx_audit_metadata,type:gin" json:"metadata,omitempty"`
	Success      *bool           `gorm:"default:true;index:idx_audit_success" json:"success"`
	ErrorMessage *string         `gorm:"type:text" json:"error_message,omitempty"`
	CreatedAt    time.Time       `gorm:"default:CURRENT_TIMESTAMP;index:idx_audit_created_at" json:"created_at"`
}

func (AuditLog) TableName() string {
	return "audit_log"
}

// Audit action constants
const (
	AuditActionSignupInitiated        = "signup_initiated"
	AuditActionSignupCompleted        = "signup_completed"
	AuditActionEmailVerified          = "email_verified"
	AuditActionMobileVerified         = "mobile_verified"
	AuditActionLoginSuccess           = "login_success"
	AuditActionLoginSuccessful        = "login_successful"
	AuditActionLoginFailed            = "login_failed"
	AuditActionLogout                 = "logout"
	AuditActionPasswordChanged        = "password_changed"
	AuditActionPasswordResetRequested = "password_reset_requested"
	AuditActionPasswordResetCompleted = "password_reset_completed"
	AuditActionPasswordResetFailed    = "password_reset_failed"
	AuditActionProfileUpdated         = "profile_updated"
	AuditActionAccountActivated       = "account_activated"
	AuditActionAccountDeactivated     = "account_deactivated"
	AuditActionSessionCreated         = "session_created"
	AuditActionSessionExpired         = "session_expired"
	AuditActionOTPGenerated           = "otp_generated"
	AuditActionOTPVerified            = "otp_verified"
	AuditActionOTPFailed              = "otp_failed"
)

// AuditLogFilter represents filter criteria for audit log queries
type AuditLogFilter struct {
	ID            *uint
	CustomerID    *uint
	Action        *string
	Success       *bool
	IPAddress     *string
	RequestID     *string
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
}

func (a *AuditLog) IsFailed() bool {
	return a.Success != nil && !*a.Success
}

func (a *AuditLog) IsSecurityEvent() bool {
	securityActions := map[string]bool{
		AuditActionLoginSuccess:       true,
		AuditActionLoginFailed:        true,
		AuditActionPasswordChanged:    true,
		AuditActionAccountActivated:   true,
		AuditActionAccountDeactivated: true,
		AuditActionOTPFailed:          true,
	}
	return securityActions[a.Action]
}
