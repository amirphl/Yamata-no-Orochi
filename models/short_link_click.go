package models

import "time"

// ShortLinkClick represents a single click event on a short link
// We keep a reference to short_links via ShortLinkID
// UserAgent and IP capture click-time context
type ShortLinkClick struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	ShortLinkID        uint       `gorm:"index:idx_short_link_clicks_short_link_id;not null" json:"short_link_id"`
	UID                *string    `gorm:"size:64;index:idx_short_link_clicks_uid" json:"uid,omitempty"`
	CampaignID         *uint      `gorm:"index:idx_short_link_clicks_campaign_id" json:"campaign_id,omitempty"`
	ClientID           *uint      `gorm:"index:idx_short_link_clicks_client_id" json:"client_id,omitempty"`
	ScenarioID         *uint      `gorm:"index:idx_short_link_clicks_scenario_id" json:"scenario_id,omitempty"`
	ScenarioName       *string    `gorm:"type:text;index:idx_short_link_clicks_scenario_name_trgm" json:"scenario_name,omitempty"`
	PhoneNumber        *string    `gorm:"size:20;index:idx_short_link_clicks_phone_number" json:"phone_number,omitempty"`
	LongLink           *string    `gorm:"type:text" json:"long_link,omitempty"`
	ShortLink          *string    `gorm:"type:text" json:"short_link,omitempty"`
	ShortLinkCreatedAt *time.Time `gorm:"column:short_link_created_at" json:"short_link_created_at,omitempty"`
	ShortLinkUpdatedAt *time.Time `gorm:"column:short_link_updated_at" json:"short_link_updated_at,omitempty"`
	UserAgent          *string    `gorm:"type:text" json:"user_agent,omitempty"`
	IP                 *string    `gorm:"size:64" json:"ip,omitempty"`
	CreatedAt          time.Time  `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC');index:idx_short_link_clicks_created_at" json:"created_at"`
}

// TableName returns the table name for ShortLinkClick
func (ShortLinkClick) TableName() string { return "short_link_clicks" }
