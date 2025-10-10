package models

import (
	"encoding/json"
	"time"

	"github.com/lib/pq"
)

// ProcessedCampaign represents a campaign prepared for sending with resolved audience
// It references the original campaign and stores the ordered audience IDs to send SMS to
// along with the last processed audience ID for resuming
// Table: processed_campaigns
// Indices: campaign_id
// Array columns use PostgreSQL biginteger[]
type ProcessedCampaign struct {
	ID             uint            `gorm:"primaryKey" json:"id"`
	CampaignID     uint            `gorm:"not null;index:idx_processed_campaigns_campaign_id" json:"campaign_id"`
	CampaignJSON   json.RawMessage `gorm:"type:jsonb;not null" json:"campaign_json"`
	AudienceIDs    pq.Int64Array   `gorm:"type:bigint[];not null" json:"audience_ids"`
	LastAudienceID *int64          `json:"last_audience_id,omitempty"`

	CreatedAt time.Time `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')" json:"created_at"`
	UpdatedAt time.Time `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')" json:"updated_at"`
}

func (ProcessedCampaign) TableName() string { return "processed_campaigns" }

// ProcessedCampaignFilter provides filter fields for repository queries
type ProcessedCampaignFilter struct {
	ID            *uint
	CampaignID    *uint
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
}
