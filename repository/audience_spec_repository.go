package repository

import (
	"context"

	"gorm.io/gorm"
)

// AudienceSpecSourceRow is one active tag joined to the latest statistics for
// its complete level1/level2/level3 hierarchy.
type AudienceSpecSourceRow struct {
	TagID          uint   `gorm:"column:tag_id"`
	Layer1Category string `gorm:"column:layer1_category"`
	Layer2Category string `gorm:"column:layer2_category"`
	Layer3Category string `gorm:"column:layer3_category"`
	StatsFound     bool   `gorm:"column:stats_found"`
	DistinctUsers  int64  `gorm:"column:distinct_users"`
	BlackUsers     int64  `gorm:"column:black_users"`
	WhiteUsers     int64  `gorm:"column:white_users"`
	PinkUsers      int64  `gorm:"column:pink_users"`
	WeakWhite      int64  `gorm:"column:weak_white"`
	GoodWhite      int64  `gorm:"column:good_white"`
	BestWhite      int64  `gorm:"column:best_white"`
	WeakBlack      int64  `gorm:"column:weak_black"`
	GoodBlack      int64  `gorm:"column:good_black"`
	BestBlack      int64  `gorm:"column:best_black"`
	WeakPink       int64  `gorm:"column:weak_pink"`
	GoodPink       int64  `gorm:"column:good_pink"`
	BestPink       int64  `gorm:"column:best_pink"`
	ScoredUsers    int64  `gorm:"column:scored_users"`
}

type AudienceSpecRepository interface {
	ListActive(ctx context.Context) ([]AudienceSpecSourceRow, error)
}

type audienceSpecRepositoryImpl struct {
	db *gorm.DB
}

func NewAudienceSpecRepository(db *gorm.DB) AudienceSpecRepository {
	return &audienceSpecRepositoryImpl{db: db}
}

// ListActive reads only active tags and joins each reference to the newest
// statistics snapshot for the exact hierarchy. PostgreSQL is the source of
// truth; no filesystem fallback is involved.
func (r *audienceSpecRepositoryImpl) ListActive(ctx context.Context) ([]AudienceSpecSourceRow, error) {
	const query = `
WITH ranked_stats AS (
    SELECT
        layer1_category,
        layer2_category,
        layer3_category,
        COALESCE(distinct_users, 0) AS distinct_users,
        COALESCE(black_users, 0) AS black_users,
        COALESCE(white_users, 0) AS white_users,
        COALESCE(pink_users, 0) AS pink_users,
        COALESCE(weak_white, 0) AS weak_white,
        COALESCE(good_white, 0) AS good_white,
        COALESCE(best_white, 0) AS best_white,
        COALESCE(weak_black, 0) AS weak_black,
        COALESCE(good_black, 0) AS good_black,
        COALESCE(best_black, 0) AS best_black,
        COALESCE(weak_pink, 0) AS weak_pink,
        COALESCE(good_pink, 0) AS good_pink,
        COALESCE(best_pink, 0) AS best_pink,
        COALESCE(scored_users, 0) AS scored_users,
        DENSE_RANK() OVER (
            PARTITION BY layer1_category, layer2_category, layer3_category
            ORDER BY calculated_at DESC NULLS LAST
        ) AS recency_rank
    FROM src_layer_all_stats
    WHERE NULLIF(BTRIM(layer1_category), '') IS NOT NULL
      AND NULLIF(BTRIM(layer2_category), '') IS NOT NULL
      AND NULLIF(BTRIM(layer3_category), '') IS NOT NULL
), latest_stats AS (
    SELECT
        layer1_category,
        layer2_category,
        layer3_category,
        distinct_users,
        black_users,
        white_users,
        pink_users,
        weak_white,
        good_white,
        best_white,
        weak_black,
        good_black,
        best_black,
        weak_pink,
        good_pink,
        best_pink,
        scored_users
    FROM ranked_stats
    WHERE recency_rank = 1
)
SELECT
    reference.id AS tag_id,
    reference.layer1_category,
    reference.layer2_category,
    reference.layer3_category,
    stats.layer1_category IS NOT NULL AS stats_found,
    COALESCE(stats.distinct_users, 0) AS distinct_users,
    COALESCE(stats.black_users, 0) AS black_users,
    COALESCE(stats.white_users, 0) AS white_users,
    COALESCE(stats.pink_users, 0) AS pink_users,
    COALESCE(stats.weak_white, 0) AS weak_white,
    COALESCE(stats.good_white, 0) AS good_white,
    COALESCE(stats.best_white, 0) AS best_white,
    COALESCE(stats.weak_black, 0) AS weak_black,
    COALESCE(stats.good_black, 0) AS good_black,
    COALESCE(stats.best_black, 0) AS best_black,
    COALESCE(stats.weak_pink, 0) AS weak_pink,
    COALESCE(stats.good_pink, 0) AS good_pink,
    COALESCE(stats.best_pink, 0) AS best_pink,
    COALESCE(stats.scored_users, 0) AS scored_users
FROM src_reference AS reference
JOIN tags AS tag
  ON tag.id = reference.id
 AND tag.is_active IS TRUE
LEFT JOIN latest_stats AS stats
  ON stats.layer1_category = reference.layer1_category
 AND stats.layer2_category = reference.layer2_category
 AND stats.layer3_category = reference.layer3_category
ORDER BY
    reference.layer1_category,
    reference.layer2_category,
    reference.layer3_category,
    reference.id`

	var rows []AudienceSpecSourceRow
	if err := r.db.WithContext(ctx).Raw(query).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
