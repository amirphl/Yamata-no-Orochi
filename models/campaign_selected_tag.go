package models

import "time"

// CampaignSelectedTag is an immutable snapshot of a tag selected for one
// campaign. Replacing a selection set deletes the old rows and inserts a new
// complete set in one transaction.
type CampaignSelectedTag struct {
	ID                            uint64   `gorm:"primaryKey" json:"id"`
	CampaignID                    uint     `gorm:"not null;uniqueIndex:uk_campaign_selected_tags_campaign_tag,priority:1;uniqueIndex:uk_campaign_selected_tags_campaign_order,priority:1;index:idx_campaign_selected_tags_campaign" json:"campaign_id"`
	BundleID                      uint     `gorm:"not null;index:idx_campaign_selected_tags_bundle_tag,priority:1" json:"bundle_id"`
	TagID                         uint     `gorm:"not null;uniqueIndex:uk_campaign_selected_tags_campaign_tag,priority:2;index:idx_campaign_selected_tags_bundle_tag,priority:2" json:"tag_id"`
	SelectionOrder                int      `gorm:"not null;uniqueIndex:uk_campaign_selected_tags_campaign_order,priority:2" json:"selection_order"`
	BundlePersonaFitScoreSnapshot *float64 `gorm:"type:numeric(5,2)" json:"bundle_persona_fit_score_snapshot"`
	TagDisplayTitleSnapshot       *string  `gorm:"type:text" json:"tag_display_title_snapshot"`
	TagAudienceCountSnapshot      *int64   `gorm:"type:bigint" json:"tag_audience_count_snapshot"`
	// CTR snapshots are nil until a per-tag CTR source is introduced. Nil means
	// "not measured"; it must not be replaced with a misleading numeric zero.
	TestPhaseAvgCTRSnapshot *float64  `gorm:"type:numeric" json:"test_phase_avg_ctr_snapshot"`
	OverallAvgCTRSnapshot   *float64  `gorm:"type:numeric" json:"overall_avg_ctr_snapshot"`
	SelectedByCustomerID    uint      `gorm:"not null;index:idx_campaign_selected_tags_selected_by_customer" json:"selected_by_customer_id"`
	CreatedAt               time.Time `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')" json:"created_at"`
	UpdatedAt               time.Time `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')" json:"updated_at"`
}

func (CampaignSelectedTag) TableName() string { return "campaign_selected_tags" }

type SmartTargetingTagRow struct {
	TagID                 uint     `gorm:"column:tag_id"`
	TagName               string   `gorm:"column:tag_name"`
	TagDisplayTitle       *string  `gorm:"column:tag_display_title"`
	TagAudiencePersona    *string  `gorm:"column:tag_audience_persona"`
	TagAudienceCount      *int64   `gorm:"column:tag_audience_count"`
	BundlePersonaFitScore *float64 `gorm:"column:bundle_persona_fit_score"`
	EvaluationRunID       *int64   `gorm:"column:evaluation_run_id"`
	FitLevel              *string  `gorm:"column:fit_level"`
	RelationType          *string  `gorm:"column:relation_type"`
	Reason                *string  `gorm:"column:reason"`
	TestPhaseAvgCTR       *float64 `gorm:"column:test_phase_avg_ctr"`
	OverallAvgCTR         *float64 `gorm:"column:overall_avg_ctr"`
	Selected              bool     `gorm:"column:selected"`
}

type CampaignSelectedTagSummary struct {
	SelectedTagCount    int64 `gorm:"column:selected_tag_count"`
	SelectedRawCapacity int64 `gorm:"column:selected_raw_capacity"`
}
