package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// SMSCampaignStatus represents the status of an SMS campaign
type SMSCampaignStatus string

const (
	SMSCampaignStatusInitiated          SMSCampaignStatus = "initiated"
	SMSCampaignStatusInProgress         SMSCampaignStatus = "in-progress"
	SMSCampaignStatusWaitingForApproval SMSCampaignStatus = "waiting-for-approval"
	SMSCampaignStatusApproved           SMSCampaignStatus = "approved"
	SMSCampaignStatusRejected           SMSCampaignStatus = "rejected"
)

// String returns the string representation of the status
func (s SMSCampaignStatus) String() string {
	return string(s)
}

// Valid checks if the status is valid
func (s SMSCampaignStatus) Valid() bool {
	switch s {
	case SMSCampaignStatusInitiated, SMSCampaignStatusInProgress,
		SMSCampaignStatusWaitingForApproval, SMSCampaignStatusApproved,
		SMSCampaignStatusRejected:
		return true
	default:
		return false
	}
}

// Scan implements the sql.Scanner interface for SMSCampaignStatus
func (s *SMSCampaignStatus) Scan(value any) error {
	if value == nil {
		*s = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*s = SMSCampaignStatus(v)
	case []byte:
		*s = SMSCampaignStatus(string(v))
	default:
		return fmt.Errorf("cannot scan %T into SMSCampaignStatus", value)
	}

	return nil
}

// Value implements the driver.Valuer interface for SMSCampaignStatus
func (s SMSCampaignStatus) Value() (driver.Value, error) {
	if !s.Valid() {
		return nil, fmt.Errorf("invalid SMSCampaignStatus: %s", s)
	}
	return string(s), nil
}

// SMSCampaignSpec represents the JSON specification for an SMS campaign
type SMSCampaignSpec struct {
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

// Value implements the driver.Valuer interface for SMSCampaignSpec
func (s SMSCampaignSpec) Value() (driver.Value, error) {
	return json.Marshal(s)
}

// Scan implements the sql.Scanner interface for SMSCampaignSpec
func (s *SMSCampaignSpec) Scan(value any) error {
	if value == nil {
		*s = SMSCampaignSpec{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into SMSCampaignSpec", value)
	}

	return json.Unmarshal(bytes, s)
}

// SMSCampaign represents an SMS campaign in the database
type SMSCampaign struct {
	ID         uint              `gorm:"primaryKey" json:"id"`
	UUID       uuid.UUID         `gorm:"type:uuid;not null;uniqueIndex:uk_sms_campaigns_uuid;index:idx_sms_campaigns_uuid" json:"uuid"`
	CustomerID uint              `gorm:"not null;index:idx_sms_campaigns_customer_id" json:"customer_id"`
	Status     SMSCampaignStatus `gorm:"type:sms_campaign_status;not null;default:'initiated';index:idx_sms_campaigns_status" json:"status"`
	CreatedAt  time.Time         `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC');index:idx_sms_campaigns_created_at" json:"created_at"`
	UpdatedAt  *time.Time        `gorm:"index:idx_sms_campaigns_updated_at" json:"updated_at,omitempty"`
	Spec       SMSCampaignSpec   `gorm:"type:jsonb;not null" json:"spec"`
	Comment    *string           `gorm:"type:text" json:"comment,omitempty"`

	// Relations
	Customer *Customer `gorm:"foreignKey:CustomerID;references:ID" json:"customer,omitempty"`
}

// TableName returns the table name for the model
func (SMSCampaign) TableName() string {
	return "sms_campaigns"
}

// BeforeCreate is called before creating a new record
func (c *SMSCampaign) BeforeCreate() error {
	if c.UUID == uuid.Nil {
		c.UUID = uuid.New()
	}
	if c.Status == "" {
		c.Status = SMSCampaignStatusInitiated
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	return nil
}

// BeforeUpdate is called before updating a record
func (c *SMSCampaign) BeforeUpdate() error {
	now := time.Now().UTC()
	c.UpdatedAt = &now
	return nil
}

// IsEditable checks if the campaign can be edited
func (c *SMSCampaign) IsEditable() bool {
	return c.Status == SMSCampaignStatusInitiated ||
		c.Status == SMSCampaignStatusInProgress
}

// IsDeletable checks if the campaign can be deleted
func (c *SMSCampaign) IsDeletable() bool {
	return false
}

// CanTransitionTo checks if the campaign can transition to the given status
func (c *SMSCampaign) CanTransitionTo(newStatus SMSCampaignStatus) bool {
	switch c.Status {
	case SMSCampaignStatusInitiated:
		return newStatus == SMSCampaignStatusInProgress ||
			newStatus == SMSCampaignStatusWaitingForApproval ||
			newStatus == SMSCampaignStatusRejected
	case SMSCampaignStatusInProgress:
		return newStatus == SMSCampaignStatusWaitingForApproval ||
			newStatus == SMSCampaignStatusRejected
	case SMSCampaignStatusWaitingForApproval:
		return newStatus == SMSCampaignStatusApproved ||
			newStatus == SMSCampaignStatusRejected
	default:
		return false
	}
}

// SMSCampaignFilter represents filter criteria for SMS campaigns
type SMSCampaignFilter struct {
	ID             *uint              `json:"id,omitempty"`
	UUID           *uuid.UUID         `json:"uuid,omitempty"`
	CustomerID     *uint              `json:"customer_id,omitempty"`
	Status         *SMSCampaignStatus `json:"status,omitempty"`
	Title          *string            `json:"title,omitempty"`
	Segment        *string            `json:"segment,omitempty"`
	Sex            *string            `json:"sex,omitempty"`
	City           *string            `json:"city,omitempty"`
	LineNumber     *string            `json:"line_number,omitempty"`
	CreatedAfter   *time.Time         `json:"created_after,omitempty"`
	CreatedBefore  *time.Time         `json:"created_before,omitempty"`
	UpdatedAfter   *time.Time         `json:"updated_after,omitempty"`
	UpdatedBefore  *time.Time         `json:"updated_before,omitempty"`
	ScheduleAfter  *time.Time         `json:"schedule_after,omitempty"`
	ScheduleBefore *time.Time         `json:"schedule_before,omitempty"`
	MinBudget      *uint64            `json:"min_budget,omitempty"`
	MaxBudget      *uint64            `json:"max_budget,omitempty"`
}

// GetStatusDisplayName returns a human-readable status name
func (c *SMSCampaign) GetStatusDisplayName() string {
	switch c.Status {
	case SMSCampaignStatusInitiated:
		return "Initiated"
	case SMSCampaignStatusInProgress:
		return "In Progress"
	case SMSCampaignStatusWaitingForApproval:
		return "Waiting for Approval"
	case SMSCampaignStatusApproved:
		return "Approved"
	case SMSCampaignStatusRejected:
		return "Rejected"
	default:
		return "Unknown"
	}
}

// GetStatusColor returns a color code for the status (for UI purposes)
func (c *SMSCampaign) GetStatusColor() string {
	switch c.Status {
	case SMSCampaignStatusInitiated:
		return "#6c757d" // gray
	case SMSCampaignStatusInProgress:
		return "#007bff" // blue
	case SMSCampaignStatusWaitingForApproval:
		return "#ffc107" // yellow
	case SMSCampaignStatusApproved:
		return "#28a745" // green
	case SMSCampaignStatusRejected:
		return "#dc3545" // red
	default:
		return "#6c757d" // gray
	}
}
