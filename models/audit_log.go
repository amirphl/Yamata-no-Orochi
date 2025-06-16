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
	CreatedAt    time.Time       `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC');index:idx_audit_created_at" json:"created_at"`
}

func (AuditLog) TableName() string {
	return "audit_log"
}

// Audit action constants
const (
	AuditActionSignupInitiated        = "signup_initiated"
	AuditActionSignupFailed           = "signup_failed"
	AuditActionSignupCompleted        = "signup_completed"
	AuditActionEmailVerified          = "email_verified"
	AuditActionMobileVerified         = "mobile_verified"
	AuditActionLoginSuccess           = "login_success"
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
	AuditActionOTPVerificationFailed  = "otp_verification_failed"
	AuditActionOTPSMSFailed           = "otp_sms_failed"
	AuditActionOTPResendFailed        = "otp_resend_failed"
	AuditActionOTPExpired             = "otp_expired"
	AuditActionOTPResent              = "otp_resent"

	// SMS Campaign actions
	AuditActionCampaignCreated        = "campaign_created"
	AuditActionCampaignCreationFailed = "campaign_creation_failed"
	AuditActionCampaignUpdated        = "campaign_updated"
	AuditActionCampaignUpdateFailed   = "campaign_update_failed"

	// Payment actions
	AuditActionWalletChargeInitiated       = "wallet_charge_initiated"
	AuditActionWalletChargeCompleted       = "wallet_charge_completed"
	AuditActionWalletChargeFailed          = "wallet_charge_failed"
	AuditActionWalletCreated               = "wallet_created"
	AuditActionPaymentCallbackProcessed    = "payment_callback_processed"
	AuditActionPaymentCompleted            = "payment_completed"
	AuditActionPaymentFailed               = "payment_failed"
	AuditActionPaymentCancelled            = "payment_cancelled"
	AuditActionPaymentExpired              = "payment_expired"
	AuditActionTransactionHistoryRetrieved = "transaction_history_retrieved"

	// Agency discount actions
	AuditActionCreateDiscountByAgencyFailed           = "create_discount_by_agency_failed"
	AuditActionCreateDiscountByAgencyCompleted        = "create_discount_by_agency_completed"
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

var SecurityActions = map[string]bool{
	AuditActionLoginSuccess:          true,
	AuditActionLoginFailed:           true,
	AuditActionPasswordChanged:       true,
	AuditActionAccountActivated:      true,
	AuditActionAccountDeactivated:    true,
	AuditActionOTPVerificationFailed: true,
}

func (a *AuditLog) IsSecurityEvent() bool {
	return SecurityActions[a.Action]
}
