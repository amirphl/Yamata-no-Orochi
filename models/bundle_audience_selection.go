package models

import (
	"time"

	"github.com/lib/pq"
)

// BundleAudienceSelection records the cumulative set of audience IDs used across
// all campaigns within a bundle, keyed by (customer_id, bundle_id).
// Each row is an immutable snapshot; the latest row per (customer_id, bundle_id)
// holds the full merged set of IDs used so far in the bundle.
type BundleAudienceSelection struct {
	ID            uint          `gorm:"primaryKey" json:"id"`
	CustomerID    uint          `gorm:"not null;index:idx_bundle_aud_sel_customer_bundle" json:"customer_id"`
	BundleID      uint          `gorm:"not null;index:idx_bundle_aud_sel_customer_bundle" json:"bundle_id"`
	CorrelationID string        `gorm:"type:varchar(128);not null;uniqueIndex:uk_bundle_aud_sel_correlation_id" json:"correlation_id"`
	AudienceIDs   pq.Int64Array `gorm:"type:bigint[];not null;default:'{}'" json:"audience_ids"`
	CreatedAt     time.Time     `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')" json:"created_at"`
}

func (BundleAudienceSelection) TableName() string { return "bundle_audience_selections" }
