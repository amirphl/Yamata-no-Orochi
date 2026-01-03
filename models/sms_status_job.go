package models

import (
	"time"

	"github.com/lib/pq"
)

// SMSStatusJob represents a scheduled job to fetch SMS delivery status
type SMSStatusJob struct {
	ID                  uint           `gorm:"primaryKey" json:"id"`
	CorrelationID       string         `gorm:"size:64;index:idx_sms_status_jobs_corr_id;not null" json:"correlation_id"`
	ProcessedCampaignID uint           `gorm:"index:idx_sms_status_jobs_processed_campaign_id;not null" json:"processed_campaign_id"`
	CustomerIDs         pq.StringArray `gorm:"type:text[];not null" json:"customer_ids"`
	RetryCount          int            `gorm:"not null;default:0" json:"retry_count"`
	ScheduledAt         time.Time      `gorm:"index:idx_sms_status_jobs_scheduled_retry;not null" json:"scheduled_at"`
	ExecutedAt          *time.Time     `json:"executed_at,omitempty"`
	Error               *string        `gorm:"type:text" json:"error,omitempty"`
	CreatedAt           time.Time      `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC');not null" json:"created_at"`
	UpdatedAt           time.Time      `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC');not null" json:"updated_at"`
}

func (SMSStatusJob) TableName() string { return "sms_status_jobs" }
