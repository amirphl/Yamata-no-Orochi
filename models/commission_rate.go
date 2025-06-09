package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CommissionRate represents the commission rate configuration for agencies
type CommissionRate struct {
	ID   uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UUID uuid.UUID `gorm:"type:uuid;uniqueIndex;not null;default:gen_random_uuid()" json:"uuid"`

	// Agency and transaction type
	AgencyID        uint   `gorm:"not null;index" json:"agency_id"`                         // Marketing agency
	TransactionType string `gorm:"type:varchar(30);not null;index" json:"transaction_type"` // Type of transaction (e.g., "campaign_creation")

	// Commission configuration
	Rate      float64 `gorm:"type:decimal(5,4);not null" json:"rate"`                  // Commission rate (e.g., 0.15 for 15%)
	MinAmount uint64  `gorm:"not null;default:0" json:"min_amount"`                    // Minimum transaction amount for commission
	MaxAmount *uint64 `gorm:"type:decimal(18,2);not null;default:0" json:"max_amount"` // Maximum transaction amount for commission (null = no limit)
	IsActive  bool    `gorm:"not null;default:true;index" json:"is_active"`            // Whether this rate is currently active

	// Rate metadata
	Description string                 `gorm:"type:text" json:"description"`
	Metadata    map[string]interface{} `gorm:"type:jsonb;default:'{}'" json:"metadata"`

	// Audit fields
	CreatedAt time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`

	// Relationships
	Agency Customer `gorm:"foreignKey:AgencyID;constraint:OnDelete:CASCADE" json:"agency,omitempty"`
}

// BeforeCreate ensures UUID is set
func (cr *CommissionRate) BeforeCreate(tx *gorm.DB) error {
	if cr.UUID == uuid.Nil {
		cr.UUID = uuid.New()
	}
	return nil
}

// IsValidForAmount checks if this commission rate is valid for a given transaction amount
func (cr *CommissionRate) IsValidForAmount(amount uint64) bool {
	if !cr.IsActive {
		return false
	}

	if amount < cr.MinAmount {
		return false
	}

	if cr.MaxAmount != nil && amount > *cr.MaxAmount {
		return false
	}

	return true
}

// CalculateCommission calculates the commission amount for a given transaction amount
func (cr *CommissionRate) CalculateCommission(amount uint64) uint64 {
	if !cr.IsValidForAmount(amount) {
		return 0
	}

	commission := float64(amount) * cr.Rate
	return uint64(commission)
}

// TableName specifies the table name for GORM
func (CommissionRate) TableName() string {
	return "commission_rates"
}

// CommissionRateFilter represents filter criteria for commission rate queries
type CommissionRateFilter struct {
	ID              *uint      `json:"id,omitempty"`
	UUID            *uuid.UUID `json:"uuid,omitempty"`
	AgencyID        *uint      `json:"agency_id,omitempty"`
	TransactionType *string    `json:"transaction_type,omitempty"`
	Rate            *float64   `json:"rate,omitempty"`
	MinAmount       *uint64    `json:"min_amount,omitempty"`
	MaxAmount       *uint64    `json:"max_amount,omitempty"`
	IsActive        *bool      `json:"is_active,omitempty"`
}
