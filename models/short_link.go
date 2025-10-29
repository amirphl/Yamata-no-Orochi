package models

import "time"

// ShortLink represents a shortened link record for tracking clicks per recipient and campaign
// UID is the short unique token that maps to the original link
// PhoneNumber is required (recipient phone), CampaignID is optional
// Clicks stores total number of clicks and defaults to 0
// UserAgent and IP are optional last-known values
type ShortLink struct {
	ID          uint    `gorm:"primaryKey" json:"id"`
	UID         string  `gorm:"size:64;not null;uniqueIndex:uk_short_links_uid;index:idx_short_links_uid" json:"uid"`
	CampaignID  *uint   `gorm:"index:idx_short_links_campaign_id" json:"campaign_id,omitempty"`
	PhoneNumber string  `gorm:"size:20;not null;index:idx_short_links_phone_number" json:"phone_number"`
	Clicks      uint64  `gorm:"type:bigint;not null;default:0" json:"clicks"`
	Link        string  `gorm:"type:text;not null" json:"link"`
	UserAgent   *string `gorm:"type:text" json:"user_agent,omitempty"`
	IP          *string `gorm:"size:64" json:"ip,omitempty"`

	CreatedAt time.Time `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC');index:idx_short_links_created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')" json:"updated_at"`
}

// TableName returns the table name for ShortLink
func (ShortLink) TableName() string { return "short_links" }

// ShortLinkFilter provides filter fields for repository queries
type ShortLinkFilter struct {
	ID            *uint
	UID           *string
	CampaignID    *uint
	PhoneNumber   *string
	ClicksMin     *uint64
	ClicksMax     *uint64
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
}
