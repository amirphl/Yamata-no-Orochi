package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CommissionStatus represents the status of a commission transaction
type CommissionStatus string

const (
	CommissionStatusPending   CommissionStatus = "pending"   // Commission calculated, waiting for distribution
	CommissionStatusPaid      CommissionStatus = "paid"      // Commission paid to agency
	CommissionStatusFailed    CommissionStatus = "failed"    // Commission distribution failed
	CommissionStatusCancelled CommissionStatus = "cancelled" // Commission cancelled
)

// CommissionType represents the type of commission
type CommissionType string

const (
	CommissionTypeCampaignCreation  CommissionType = "campaign_creation"  // Commission from SMS campaign creation
	CommissionTypeCampaignRejection CommissionType = "campaign_rejection" // Commission adjustment from rejected campaigns
	CommissionTypeReferral          CommissionType = "referral"           // Commission from user referrals
	CommissionTypeService           CommissionType = "service"            // Commission from other services
)

// AgencyCommission represents commission distribution to agencies
type AgencyCommission struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UUID          uuid.UUID `gorm:"type:uuid;uniqueIndex;not null;default:gen_random_uuid()" json:"uuid"`
	CorrelationID uuid.UUID `gorm:"type:uuid;index;not null" json:"correlation_id"` // Links to source transaction

	// Agency and customer information
	AgencyID   uint `gorm:"not null;index" json:"agency_id"`   // Marketing agency receiving commission
	CustomerID uint `gorm:"not null;index" json:"customer_id"` // Customer who generated the commission
	WalletID   uint `gorm:"not null;index" json:"wallet_id"`   // Agency's wallet for receiving commission

	// Commission details
	Type       CommissionType   `gorm:"type:varchar(30);not null;index" json:"type"`
	Status     CommissionStatus `gorm:"type:varchar(20);not null;default:'pending';index" json:"status"`
	Amount     uint64           `gorm:"not null" json:"amount"`                       // Commission amount in Rials
	Percentage float64          `gorm:"type:decimal(5,4);not null" json:"percentage"` // Commission percentage (e.g., 0.15 for 15%)
	BaseAmount uint64           `gorm:"not null" json:"base_amount"`                  // Original transaction amount that generated commission

	// Source transaction information
	SourceTransactionID uint  `gorm:"index" json:"source_transaction_id"` // Transaction that generated commission
	SourceCampaignID    *uint `gorm:"index" json:"source_campaign_id"`    // SMS campaign if applicable

	// Commission metadata
	Description string                 `gorm:"type:text" json:"description"`
	Metadata    map[string]interface{} `gorm:"type:jsonb;default:'{}'" json:"metadata"`

	// Payment tracking
	PaidAt               *time.Time `gorm:"index" json:"paid_at"`                // When commission was paid
	PaymentTransactionID *uint      `gorm:"index" json:"payment_transaction_id"` // Transaction that paid the commission

	// Audit fields
	CreatedAt time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`

	// Relationships
	Agency             Customer     `gorm:"foreignKey:AgencyID;constraint:OnDelete:CASCADE" json:"agency,omitempty"`
	Customer           Customer     `gorm:"foreignKey:CustomerID;constraint:OnDelete:CASCADE" json:"customer,omitempty"`
	Wallet             Wallet       `gorm:"foreignKey:WalletID;constraint:OnDelete:CASCADE" json:"wallet,omitempty"`
	SourceTransaction  Transaction  `gorm:"foreignKey:SourceTransactionID;constraint:OnDelete:SET NULL" json:"source_transaction,omitempty"`
	SourceCampaign     SMSCampaign  `gorm:"foreignKey:SourceCampaignID;constraint:OnDelete:SET NULL" json:"source_campaign,omitempty"`
	PaymentTransaction *Transaction `gorm:"foreignKey:PaymentTransactionID;constraint:OnDelete:SET NULL" json:"payment_transaction,omitempty"`
}

// BeforeCreate ensures UUID and CorrelationID are set
func (ac *AgencyCommission) BeforeCreate(tx *gorm.DB) error {
	if ac.UUID == uuid.Nil {
		ac.UUID = uuid.New()
	}
	if ac.CorrelationID == uuid.Nil {
		ac.CorrelationID = uuid.New()
	}
	return nil
}

// IsPaid returns true if the commission has been paid
func (ac *AgencyCommission) IsPaid() bool {
	return ac.Status == CommissionStatusPaid
}

// IsPending returns true if the commission is still pending
func (ac *AgencyCommission) IsPending() bool {
	return ac.Status == CommissionStatusPending
}

// CanBePaid returns true if the commission can be paid
func (ac *AgencyCommission) CanBePaid() bool {
	return ac.Status == CommissionStatusPending
}

// MarkAsPaid marks the commission as paid and sets payment details
func (ac *AgencyCommission) MarkAsPaid(paymentTransactionID uint) {
	ac.Status = CommissionStatusPaid
	ac.PaidAt = &time.Time{}
	*ac.PaidAt = time.Now()
	ac.PaymentTransactionID = &paymentTransactionID
}

// AgencyCommissionFilter represents filter criteria for agency commission queries
type AgencyCommissionFilter struct {
	ID                   *uint             `json:"id,omitempty"`
	UUID                 *uuid.UUID        `json:"uuid,omitempty"`
	CorrelationID        *uuid.UUID        `json:"correlation_id,omitempty"`
	AgencyID             *uint             `json:"agency_id,omitempty"`
	CustomerID           *uint             `json:"customer_id,omitempty"`
	WalletID             *uint             `json:"wallet_id,omitempty"`
	Type                 *CommissionType   `json:"type,omitempty"`
	Status               *CommissionStatus `json:"status,omitempty"`
	Amount               *uint64           `json:"amount,omitempty"`
	SourceTransactionID  *uint             `json:"source_transaction_id,omitempty"`
	SourceCampaignID     *uint             `json:"source_campaign_id,omitempty"`
	PaymentTransactionID *uint             `json:"payment_transaction_id,omitempty"`
	CreatedAfter         *time.Time        `json:"created_after,omitempty"`
	CreatedBefore        *time.Time        `json:"created_before,omitempty"`
	PaidAfter            *time.Time        `json:"paid_after,omitempty"`
	PaidBefore           *time.Time        `json:"paid_before,omitempty"`
}
