package models

import "time"

// SMSStatusResult stores provider status metrics for a given job/customer
type SMSStatusResult struct {
	ID                    uint   `gorm:"primaryKey" json:"id"`
	JobID                 uint   `gorm:"index:idx_sms_status_results_job_id;not null" json:"job_id"`
	ProcessedCampaignID   uint   `gorm:"index:idx_sms_status_results_processed_campaign_id;not null" json:"processed_campaign_id"`
	CustomerID            string `gorm:"index:idx_sms_status_results_customer_id;not null" json:"customer_id"`
	ServerID              *string
	TotalParts            *int64
	TotalDeliveredParts   *int64
	TotalUndeliveredParts *int64
	TotalUnknownParts     *int64
	Status                *string
	CreatedAt             time.Time `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC');not null" json:"created_at"`
}

func (SMSStatusResult) TableName() string { return "sms_status_results" }
