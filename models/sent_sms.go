package models

import "time"

// SMSSendStatus enumerates status of a sent SMS record
type SMSSendStatus string

const (
	SMSSendStatusPending      SMSSendStatus = "pending"
	SMSSendStatusSuccessful   SMSSendStatus = "successful"
	SMSSendStatusUnsuccessful SMSSendStatus = "unsuccessful"
)

// SentSMS records a single phone number delivery attempt under a processed campaign
type SentSMS struct {
	ID                  uint          `gorm:"primaryKey" json:"id"`
	ProcessedCampaignID uint          `gorm:"not null;index:idx_sent_sms_processed_campaign_id" json:"processed_campaign_id"`
	PhoneNumber         string        `gorm:"size:20;not null;index:idx_sent_sms_phone_number" json:"phone_number"`
	TrackingID          string        `gorm:"size:64;not null;index:idx_sent_sms_tracking_id" json:"tracking_id"`
	PartsDelivered      int           `gorm:"default:0" json:"parts_delivered"`
	Status              SMSSendStatus `gorm:"type:sent_sms_status;not null;default:'pending';index:idx_sent_sms_status" json:"status"`
	CreatedAt           time.Time     `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC');index:idx_sent_sms_created_at" json:"created_at"`
	UpdatedAt           time.Time     `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')" json:"updated_at"`
}

func (SentSMS) TableName() string { return "sent_sms" }

// SentSMSFilter provides filter fields for repository queries
type SentSMSFilter struct {
	ID                  *uint
	ProcessedCampaignID *uint
	PhoneNumber         *string
	Status              *SMSSendStatus
	CreatedAfter        *time.Time
	CreatedBefore       *time.Time
}
