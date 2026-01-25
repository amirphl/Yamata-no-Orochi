package models

import "time"

// SegmentPriceFactor stores a configurable price factor for a level3 value.
// Level3 is not unique; the latest row (by created_at) should be used when multiple exist.
// Table: segment_price_factors
type SegmentPriceFactor struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Level3      string    `gorm:"size:255;not null;index:idx_segment_price_factors_level3" json:"level3"`
	PriceFactor float64   `gorm:"type:numeric(10,4);not null" json:"price_factor"`
	CreatedAt   time.Time `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC');index:idx_segment_price_factors_created_at" json:"created_at"`
	UpdatedAt   time.Time `gorm:"default:(CURRENT_TIMESTAMP AT TIME ZONE 'UTC')" json:"updated_at"`
}

func (SegmentPriceFactor) TableName() string {
	return "segment_price_factors"
}

type SegmentPriceFactorFilter struct {
	Level3 *string `json:"level3,omitempty"`
}
