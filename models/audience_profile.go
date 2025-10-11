package models

import (
	"time"

	"github.com/lib/pq"
)

// AudienceProfile represents a profile of an audience used for campaign targeting
// Arrays are stored as PostgreSQL TEXT[] columns
// PhoneNumber is optional and unique when present
// UID is a required unique identifier (external/system specific)
type AudienceProfile struct {
	ID          int64         `gorm:"primaryKey;autoIncrement;type:bigserial" json:"id"`
	UID         string        `gorm:"size:255;not null;uniqueIndex:uk_audience_profiles_uid;index:idx_audience_profiles_uid" json:"uid"`
	PhoneNumber *string       `gorm:"size:20;uniqueIndex:uk_audience_profiles_phone_number;index:idx_audience_profiles_phone_number" json:"phone_number,omitempty"`
	Tags        pq.Int32Array `gorm:"type:integer[];index:idx_audience_profiles_tag_gin,using:gin" json:"tags"`
	Color       string        `gorm:"size:20;not null;index:idx_audience_profiles_color" json:"color"`

	CreatedAt time.Time `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC');index:idx_audience_profiles_created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')" json:"updated_at"`
}

func (AudienceProfile) TableName() string {
	return "audience_profiles"
}

// AudienceProfileFilter represents filter criteria for audience profile queries
// For array fields, use the *Contains filters to match any single value presence
type AudienceProfileFilter struct {
	ID            *uint
	UID           *string
	PhoneNumber   *string
	Tags          *pq.Int32Array
	Color         *string
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
}
