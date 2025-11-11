package models

import "time"

// ShortLinkClick represents a single click event on a short link
// We keep a reference to short_links via ShortLinkID
// UserAgent and IP capture click-time context
type ShortLinkClick struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	ShortLinkID uint      `gorm:"index:idx_short_link_clicks_short_link_id;not null" json:"short_link_id"`
	UserAgent   *string   `gorm:"type:text" json:"user_agent,omitempty"`
	IP          *string   `gorm:"size:64" json:"ip,omitempty"`
	CreatedAt   time.Time `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC');index:idx_short_link_clicks_created_at" json:"created_at"`
}

// TableName returns the table name for ShortLinkClick
func (ShortLinkClick) TableName() string { return "short_link_clicks" }
