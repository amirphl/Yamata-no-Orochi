package repository

import (
	"context"
	"fmt"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

// SmartTargetingAudienceQuery is platform-independent by design. Capacity
// jobs count this population; schedulers select it only when execution starts.
type SmartTargetingAudienceQuery struct {
	BundleID     uint
	TagIDs       []int64
	ScoreClasses []string
}

type SmartTargetingAudienceRepository interface {
	CalculateCapacity(ctx context.Context, query SmartTargetingAudienceQuery) (*SmartTargetingAudienceCapacity, error)
	SelectCandidates(ctx context.Context, query SmartTargetingAudienceQuery, limit int64) ([]*models.AudienceProfile, error)
	SelectRandomForTag(ctx context.Context, query SmartTargetingAudienceQuery, tagID int64, excludeAudienceIDs []int64, limit int64) ([]*models.AudienceProfile, error)
}

type SmartTargetingAudienceCapacity struct {
	RawUniqueCount      int64 `gorm:"column:raw_unique_count"`
	EligibleUniqueCount int64 `gorm:"column:eligible_unique_count"`
}

type smartTargetingAudienceRepository struct{ db *gorm.DB }

func NewSmartTargetingAudienceRepository(db *gorm.DB) SmartTargetingAudienceRepository {
	return &smartTargetingAudienceRepository{db: db}
}

func (r *smartTargetingAudienceRepository) getDB(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(TxContextKey).(*gorm.DB); ok && tx != nil {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

const smartTargetingBasePopulationCTE = `
WITH tagged_population AS (
    SELECT ap.id, ap.uid, ap.phone_number, ap.tags, ap.normalized_score
    FROM audience_profiles AS ap
    WHERE ap.tags && ?::integer[]
      AND ap.phone_number IS NOT NULL
      AND BTRIM(ap.phone_number) <> ''
), candidate_population AS (
    SELECT tagged.*
    FROM tagged_population AS tagged
    WHERE NOT EXISTS (
          SELECT 1
          FROM bundle_audience_selection_members AS used
          WHERE used.bundle_id = ? AND used.audience_id = tagged.id
      )
)`

const smartTargetingClassifiedPopulationCTE = `, bounds AS (
    SELECT percentile_disc(0.33) WITHIN GROUP (ORDER BY normalized_score) AS p33,
           percentile_disc(0.66) WITHIN GROUP (ORDER BY normalized_score) AS p66
    FROM candidate_population
    WHERE normalized_score IS NOT NULL
), classified AS (
    SELECT population.*,
        CASE
            WHEN population.normalized_score IS NULL THEN 'unscored'
            WHEN bounds.p33 IS NULL OR bounds.p66 IS NULL THEN 'unscored'
            WHEN population.normalized_score <= bounds.p33 THEN 'C'
            WHEN population.normalized_score <= bounds.p66 THEN 'B'
            ELSE 'A'
        END AS score_class
    FROM candidate_population AS population
    CROSS JOIN bounds
)`

const smartTargetingPopulationCTE = smartTargetingBasePopulationCTE + smartTargetingClassifiedPopulationCTE

func smartTargetingAllClasses(classes []string) bool {
	return len(classes) == 3 && classes[0] == "A" && classes[1] == "B" && classes[2] == "C"
}

func (r *smartTargetingAudienceRepository) CalculateCapacity(ctx context.Context, query SmartTargetingAudienceQuery) (*SmartTargetingAudienceCapacity, error) {
	if query.BundleID == 0 || len(query.TagIDs) == 0 || len(query.ScoreClasses) == 0 {
		return nil, fmt.Errorf("invalid smart-targeting audience count query")
	}
	var capacity SmartTargetingAudienceCapacity
	if smartTargetingAllClasses(query.ScoreClasses) {
		sql := smartTargetingBasePopulationCTE + `
SELECT (SELECT COUNT(*) FROM tagged_population) AS raw_unique_count,
       (SELECT COUNT(*) FROM candidate_population) AS eligible_unique_count`
		err := r.getDB(ctx).Raw(sql, pq.Array(query.TagIDs), query.BundleID).Scan(&capacity).Error
		return &capacity, err
	}
	sql := smartTargetingPopulationCTE + `
SELECT (SELECT COUNT(*) FROM tagged_population) AS raw_unique_count,
       COUNT(*) FILTER (WHERE (?::boolean OR score_class = ANY(?::text[]))) AS eligible_unique_count
FROM classified`
	err := r.getDB(ctx).Raw(sql, pq.Array(query.TagIDs), query.BundleID,
		smartTargetingAllClasses(query.ScoreClasses), pq.Array(query.ScoreClasses)).Scan(&capacity).Error
	return &capacity, err
}

func (r *smartTargetingAudienceRepository) SelectCandidates(ctx context.Context, query SmartTargetingAudienceQuery, limit int64) ([]*models.AudienceProfile, error) {
	if query.BundleID == 0 || len(query.TagIDs) == 0 || len(query.ScoreClasses) == 0 || limit <= 0 {
		return nil, fmt.Errorf("invalid smart-targeting audience selection query")
	}
	var rows []*models.AudienceProfile
	if smartTargetingAllClasses(query.ScoreClasses) {
		sql := smartTargetingBasePopulationCTE + `
SELECT id, uid, phone_number, tags, normalized_score
FROM candidate_population
ORDER BY normalized_score DESC NULLS LAST, id ASC
LIMIT ?`
		err := r.getDB(ctx).Raw(sql, pq.Array(query.TagIDs), query.BundleID, limit).Scan(&rows).Error
		return rows, err
	}
	sql := smartTargetingPopulationCTE + `
SELECT id, uid, phone_number, tags, normalized_score
FROM classified
WHERE (?::boolean OR score_class = ANY(?::text[]))
ORDER BY normalized_score DESC NULLS LAST, id ASC
LIMIT ?`
	err := r.getDB(ctx).Raw(sql, pq.Array(query.TagIDs), query.BundleID,
		smartTargetingAllClasses(query.ScoreClasses), pq.Array(query.ScoreClasses), limit).Scan(&rows).Error
	return rows, err
}

// SelectRandomForTag applies the same union-wide score classification used by
// exact capacity, then samples only one tag. The exclusion list contains only
// audiences allocated to earlier satisfied tags, so an insufficient tag does
// not consume candidates needed by later tags.
func (r *smartTargetingAudienceRepository) SelectRandomForTag(ctx context.Context, query SmartTargetingAudienceQuery, tagID int64, excludeAudienceIDs []int64, limit int64) ([]*models.AudienceProfile, error) {
	if query.BundleID == 0 || len(query.TagIDs) == 0 || len(query.ScoreClasses) == 0 || tagID <= 0 || limit <= 0 {
		return nil, fmt.Errorf("invalid smart-targeting per-tag sample query")
	}
	if excludeAudienceIDs == nil {
		excludeAudienceIDs = []int64{}
	}
	var rows []*models.AudienceProfile
	if smartTargetingAllClasses(query.ScoreClasses) {
		sql := smartTargetingBasePopulationCTE + `
SELECT id, uid, phone_number, tags, normalized_score
FROM candidate_population
WHERE ?::integer = ANY(tags)
  AND NOT (id = ANY(?::bigint[]))
ORDER BY RANDOM()
LIMIT ?`
		err := r.getDB(ctx).Raw(sql, pq.Array(query.TagIDs), query.BundleID, tagID, pq.Array(excludeAudienceIDs), limit).Scan(&rows).Error
		return rows, err
	}
	sql := smartTargetingPopulationCTE + `
SELECT id, uid, phone_number, tags, normalized_score
FROM classified
WHERE (?::boolean OR score_class = ANY(?::text[]))
  AND ?::integer = ANY(tags)
  AND NOT (id = ANY(?::bigint[]))
ORDER BY RANDOM()
LIMIT ?`
	err := r.getDB(ctx).Raw(sql, pq.Array(query.TagIDs), query.BundleID,
		smartTargetingAllClasses(query.ScoreClasses), pq.Array(query.ScoreClasses), tagID, pq.Array(excludeAudienceIDs), limit).Scan(&rows).Error
	return rows, err
}
