package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CampaignStatus represents the status of an campaign
type CampaignStatus string

const (
	CampaignStatusInitiated          CampaignStatus = "initiated"
	CampaignStatusInProgress         CampaignStatus = "in-progress"
	CampaignStatusWaitingForApproval CampaignStatus = "waiting-for-approval"
	CampaignStatusApproved           CampaignStatus = "approved"
	CampaignStatusRejected           CampaignStatus = "rejected"
)

// String returns the string representation of the status
func (s CampaignStatus) String() string {
	return string(s)
}

// Valid checks if the status is valid
func (s CampaignStatus) Valid() bool {
	switch s {
	case CampaignStatusInitiated, CampaignStatusInProgress,
		CampaignStatusWaitingForApproval, CampaignStatusApproved,
		CampaignStatusRejected:
		return true
	default:
		return false
	}
}

// Scan implements the sql.Scanner interface for CampaignStatus
func (s *CampaignStatus) Scan(value any) error {
	if value == nil {
		*s = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*s = CampaignStatus(v)
	case []byte:
		*s = CampaignStatus(string(v))
	default:
		return fmt.Errorf("cannot scan %T into CampaignStatus", value)
	}

	return nil
}

// Value implements the driver.Valuer interface for CampaignStatus
func (s CampaignStatus) Value() (driver.Value, error) {
	if !s.Valid() {
		return nil, fmt.Errorf("invalid CampaignStatus: %s", s)
	}
	return string(s), nil
}

// CampaignSpec represents the JSON specification for an campaign
type CampaignSpec struct {
	// Campaign details
	Title *string `json:"title,omitempty"`

	// Target audience
	Segment    *string  `json:"segment,omitempty"`
	Subsegment []string `json:"subsegment,omitempty"`
	Sex        *string  `json:"sex,omitempty"`
	City       []string `json:"city,omitempty"`

	// Campaign content
	AdLink  *string `json:"adlink,omitempty"`
	Content *string `json:"content,omitempty"`

	// Scheduling and configuration
	ScheduleAt *time.Time `json:"schedule_at,omitempty"`
	LineNumber *string    `json:"line_number,omitempty"`

	// Budget
	Budget *uint64 `json:"budget,omitempty"`
}

// Value implements the driver.Valuer interface for CampaignSpec
func (s CampaignSpec) Value() (driver.Value, error) {
	return json.Marshal(s)
}

// Scan implements the sql.Scanner interface for CampaignSpec
func (s *CampaignSpec) Scan(value any) error {
	if value == nil {
		*s = CampaignSpec{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into CampaignSpec", value)
	}

	return json.Unmarshal(bytes, s)
}

// Campaign represents an campaign in the database
type Campaign struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	UUID       uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uk_sms_campaigns_uuid;index:idx_sms_campaigns_uuid" json:"uuid"`
	CustomerID uint           `gorm:"not null;index:idx_sms_campaigns_customer_id" json:"customer_id"`
	Status     CampaignStatus `gorm:"type:sms_campaign_status;not null;default:'initiated';index:idx_sms_campaigns_status" json:"status"`
	CreatedAt  time.Time      `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC');index:idx_sms_campaigns_created_at" json:"created_at"`
	UpdatedAt  *time.Time     `gorm:"index:idx_sms_campaigns_updated_at" json:"updated_at,omitempty"`
	Spec       CampaignSpec   `gorm:"type:jsonb;not null" json:"spec"`
	Comment    *string        `gorm:"type:text" json:"comment,omitempty"`

	// Relations
	Customer          *Customer          `gorm:"foreignKey:CustomerID;references:ID" json:"customer,omitempty"`
	AgencyCommissions []AgencyCommission `gorm:"foreignKey:SourceCampaignID" json:"agency_commissions,omitempty"`
}

// TableName returns the table name for the model
func (Campaign) TableName() string {
	return "sms_campaigns"
}

// BeforeCreate is called before creating a new record
func (c *Campaign) BeforeCreate(tx *gorm.DB) error {
	if c.UUID == uuid.Nil {
		c.UUID = uuid.New()
	}
	if c.Status == "" {
		c.Status = CampaignStatusInitiated
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = utils.UTCNow()
	}
	return nil
}

// BeforeUpdate is called before updating a record
func (c *Campaign) BeforeUpdate(tx *gorm.DB) error {
	now := utils.UTCNow()
	c.UpdatedAt = &now
	return nil
}

// IsEditable checks if the campaign can be edited
func (c *Campaign) IsEditable() bool {
	return c.Status == CampaignStatusInitiated ||
		c.Status == CampaignStatusInProgress
}

// IsDeletable checks if the campaign can be deleted
func (c *Campaign) IsDeletable() bool {
	return false
}

// CanTransitionTo checks if the campaign can transition to the given status
func (c *Campaign) CanTransitionTo(newStatus CampaignStatus) bool {
	switch c.Status {
	case CampaignStatusInitiated:
		return newStatus == CampaignStatusInProgress ||
			newStatus == CampaignStatusWaitingForApproval ||
			newStatus == CampaignStatusRejected
	case CampaignStatusInProgress:
		return newStatus == CampaignStatusWaitingForApproval ||
			newStatus == CampaignStatusRejected
	case CampaignStatusWaitingForApproval:
		return newStatus == CampaignStatusApproved ||
			newStatus == CampaignStatusRejected
	default:
		return false
	}
}

// CampaignFilter represents filter criteria for campaigns
type CampaignFilter struct {
	ID             *uint           `json:"id,omitempty"`
	UUID           *uuid.UUID      `json:"uuid,omitempty"`
	CustomerID     *uint           `json:"customer_id,omitempty"`
	Status         *CampaignStatus `json:"status,omitempty"`
	Title          *string         `json:"title,omitempty"`
	Segment        *string         `json:"segment,omitempty"`
	Sex            *string         `json:"sex,omitempty"`
	City           *string         `json:"city,omitempty"`
	LineNumber     *string         `json:"line_number,omitempty"`
	CreatedAfter   *time.Time      `json:"created_after,omitempty"`
	CreatedBefore  *time.Time      `json:"created_before,omitempty"`
	UpdatedAfter   *time.Time      `json:"updated_after,omitempty"`
	UpdatedBefore  *time.Time      `json:"updated_before,omitempty"`
	ScheduleAfter  *time.Time      `json:"schedule_after,omitempty"`
	ScheduleBefore *time.Time      `json:"schedule_before,omitempty"`
	MinBudget      *uint64         `json:"min_budget,omitempty"`
	MaxBudget      *uint64         `json:"max_budget,omitempty"`
}

// GetStatusDisplayName returns a human-readable status name
func (c *Campaign) GetStatusDisplayName() string {
	switch c.Status {
	case CampaignStatusInitiated:
		return "Initiated"
	case CampaignStatusInProgress:
		return "In Progress"
	case CampaignStatusWaitingForApproval:
		return "Waiting for Approval"
	case CampaignStatusApproved:
		return "Approved"
	case CampaignStatusRejected:
		return "Rejected"
	default:
		return "Unknown"
	}
}

// GetStatusColor returns a color code for the status (for UI purposes)
func (c *Campaign) GetStatusColor() string {
	switch c.Status {
	case CampaignStatusInitiated:
		return "#6c757d" // gray
	case CampaignStatusInProgress:
		return "#007bff" // blue
	case CampaignStatusWaitingForApproval:
		return "#ffc107" // yellow
	case CampaignStatusApproved:
		return "#28a745" // green
	case CampaignStatusRejected:
		return "#dc3545" // red
	default:
		return "#6c757d" // gray
	}
}
