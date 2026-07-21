package models

import (
	"time"

	"github.com/lib/pq"
)

// BundleAudienceSelection is one immutable, bundle-scoped campaign allocation.
// Cross-campaign uniqueness and the exact retry set are stored once in
// BundleAudienceSelectionMember. SelectedAudienceIDs is a repository-populated
// transient projection, not another persisted copy of the allocation.
type BundleAudienceSelection struct {
	ID                  uint          `gorm:"primaryKey" json:"id"`
	CustomerID          uint          `gorm:"not null;index:idx_bundle_aud_sel_customer_bundle" json:"customer_id"`
	BundleID            uint          `gorm:"not null;index:idx_bundle_aud_sel_customer_bundle" json:"bundle_id"`
	CampaignID          *uint         `gorm:"uniqueIndex:uk_bundle_aud_sel_campaign" json:"campaign_id,omitempty"`
	CorrelationID       string        `gorm:"type:varchar(128);not null;uniqueIndex:uk_bundle_aud_sel_correlation_id" json:"correlation_id"`
	SelectedAudienceIDs pq.Int64Array `gorm:"-" json:"selected_audience_ids"`
	AudienceCount       int64         `gorm:"not null" json:"audience_count"`
	CreatedAt           time.Time     `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')" json:"created_at"`
}

func (BundleAudienceSelection) TableName() string { return "bundle_audience_selections" }

// BundleAudienceSelectionMember is the normalized append-only uniqueness
// ledger. The (bundle_id, audience_id) key makes duplicate audience assignment
// impossible even if multiple scheduler replicas race.
type BundleAudienceSelectionMember struct {
	ID          int64     `gorm:"primaryKey;autoIncrement;type:bigserial" json:"id"`
	SelectionID uint      `gorm:"not null;uniqueIndex:uk_bundle_aud_sel_member_selection_audience,priority:1" json:"selection_id"`
	BundleID    uint      `gorm:"not null;uniqueIndex:uk_bundle_aud_sel_member_bundle_audience,priority:1" json:"bundle_id"`
	AudienceID  int64     `gorm:"not null;uniqueIndex:uk_bundle_aud_sel_member_selection_audience,priority:2;uniqueIndex:uk_bundle_aud_sel_member_bundle_audience,priority:2" json:"audience_id"`
	CreatedAt   time.Time `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')" json:"created_at"`
}

func (BundleAudienceSelectionMember) TableName() string {
	return "bundle_audience_selection_members"
}
