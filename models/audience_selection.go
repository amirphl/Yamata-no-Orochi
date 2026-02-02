package models

import (
	"time"

	"github.com/lib/pq"
)

// AudienceSelection stores a snapshot of assigned audience IDs for a given customer and tag hash.
// Each row is immutable and correlated by correlation_id to the processing run that produced it.
type AudienceSelection struct {
	ID            uint          `gorm:"primaryKey" json:"id"`
	CustomerID    uint          `gorm:"not null;index:idx_audience_selections_customer_tags_created" json:"customer_id"`
	TagsHash      string        `gorm:"type:varchar(128);not null;index:idx_audience_selections_customer_tags_created" json:"tags_hash"`
	CorrelationID string        `gorm:"type:varchar(128);not null;uniqueIndex:uk_audience_selections_correlation_id" json:"correlation_id"`
	AudienceIDs   pq.Int64Array `gorm:"type:bigint[];not null;default:'{}'" json:"audience_ids"`
	CreatedAt     time.Time     `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')" json:"created_at"`
}

func (AudienceSelection) TableName() string { return "audience_selections" }
