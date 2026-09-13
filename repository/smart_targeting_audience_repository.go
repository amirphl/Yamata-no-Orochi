package repository

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	mathrand "math/rand"
	"strconv"
	"strings"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

// SmartTargetingAudienceQuery describes the eligibility rules shared by exact
// capacity, Test preview, and final selection. AllowedColors is empty for
// platforms without a delivery-color restriction.
type SmartTargetingAudienceQuery struct {
	BundleID uint
	// ApplyBundleAudienceExclusions enables the manually populated, bundle-
	// scoped exclusion list for Smart Targeting Test capacity, preview, and
	// final selection.
	ApplyBundleAudienceExclusions bool
	TagIDs                        []int64
	ScoreClasses                  []string
	AllowedColors                 []string
	// SamplingSeed is the required persisted Test-preview input hash. Using it
	// for both preview and execution makes the randomized per-tag allocation
	// repeatable while the eligible database population remains unchanged.
	SamplingSeed string
}

type SmartTargetingAudienceRepository interface {
	CalculateCapacity(ctx context.Context, query SmartTargetingAudienceQuery) (*SmartTargetingAudienceCapacity, error)
	SelectCandidates(ctx context.Context, query SmartTargetingAudienceQuery, limit int64) ([]*models.AudienceProfile, error)
	CalculateScoreBounds(ctx context.Context, query SmartTargetingAudienceQuery) (*SmartTargetingScoreBounds, error)
	SelectIDsForTag(ctx context.Context, query SmartTargetingAudienceQuery, bounds *SmartTargetingScoreBounds, tagID int64, excludeAudienceIDs []int64, limit int64) ([]int64, error)
	SelectForTag(ctx context.Context, query SmartTargetingAudienceQuery, bounds *SmartTargetingScoreBounds, tagID int64, excludeAudienceIDs []int64, limit int64) ([]*models.AudienceProfile, error)
}

type SmartTargetingAudienceCapacity struct {
	RawUniqueCount      int64 `gorm:"column:raw_unique_count"`
	EligibleUniqueCount int64 `gorm:"column:eligible_unique_count"`
}

// SmartTargetingScoreBounds contains the union-wide score boundaries used by
// every per-tag selection in one operation. Nil boundaries mean the eligible
// union has no scored profiles, so every restricted score class is empty.
type SmartTargetingScoreBounds struct {
	P33 *float64 `gorm:"column:p33"`
	P66 *float64 `gorm:"column:p66"`
}

type smartTargetingSampleRow struct {
	ID              int64         `gorm:"column:id"`
	UID             string        `gorm:"column:uid"`
	PhoneNumber     *string       `gorm:"column:phone_number"`
	Tags            pq.Int32Array `gorm:"column:tags;type:integer[]"`
	NormalizedScore *float64      `gorm:"column:normalized_score"`
	SampleKey       int64         `gorm:"column:sample_key"`
}

type smartTargetingSampleCursor struct {
	key int64
	id  int64
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
    SELECT ap.id, ap.normalized_score
    FROM audience_profiles AS ap
    WHERE ap.tags && ?::integer[]
      AND ap.phone_number IS NOT NULL
      AND BTRIM(ap.phone_number) <> ''
      AND (?::boolean OR ap.color = ANY(?::text[]))
), candidate_population AS (
    SELECT tagged.*
    FROM tagged_population AS tagged
    WHERE NOT EXISTS (
          SELECT 1
          FROM bundle_audience_selection_members AS used
          WHERE used.bundle_id = ? AND used.audience_id = tagged.id
      )
      AND (?::boolean OR NOT EXISTS (
          SELECT 1
          FROM bundle_audience_exclusions AS bundle_exclusion
          WHERE bundle_exclusion.bundle_id = ?
            AND bundle_exclusion.audience_id = tagged.id
      ))
)`

// Execution selection needs delivery columns. Capacity and score-bound work
// use the slim CTE above so PostgreSQL does not materialize UID, phone, or tag
// arrays for count/percentile-only operations.
const smartTargetingSelectionBasePopulationCTE = `
WITH tagged_population AS (
    SELECT ap.id, ap.uid, ap.phone_number, ap.tags, ap.normalized_score
    FROM audience_profiles AS ap
    WHERE ap.tags && ?::integer[]
      AND ap.phone_number IS NOT NULL
      AND BTRIM(ap.phone_number) <> ''
      AND (?::boolean OR ap.color = ANY(?::text[]))
), candidate_population AS (
    SELECT tagged.*
    FROM tagged_population AS tagged
    WHERE NOT EXISTS (
          SELECT 1
          FROM bundle_audience_selection_members AS used
          WHERE used.bundle_id = ? AND used.audience_id = tagged.id
      )
      AND (?::boolean OR NOT EXISTS (
          SELECT 1
          FROM bundle_audience_exclusions AS bundle_exclusion
          WHERE bundle_exclusion.bundle_id = ?
            AND bundle_exclusion.audience_id = tagged.id
      ))
)`

const smartTargetingClassifiedPopulationCTE = `, percentile_bounds AS (
    SELECT percentile_disc(ARRAY[0.33, 0.66]::double precision[])
               WITHIN GROUP (ORDER BY normalized_score) AS percentile_values
    FROM candidate_population
    WHERE normalized_score IS NOT NULL
), bounds AS (
    SELECT percentile_values[1] AS p33, percentile_values[2] AS p66
    FROM percentile_bounds
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
const smartTargetingSelectionPopulationCTE = smartTargetingSelectionBasePopulationCTE + smartTargetingClassifiedPopulationCTE

func smartTargetingAllClasses(classes []string) bool {
	return len(classes) == 3 && classes[0] == "A" && classes[1] == "B" && classes[2] == "C"
}

func smartTargetingPopulationArgs(query SmartTargetingAudienceQuery) []any {
	colors := query.AllowedColors
	if colors == nil {
		colors = []string{}
	}
	return []any{
		pq.Array(query.TagIDs),
		len(colors) == 0,
		pq.Array(colors),
		query.BundleID,
		!query.ApplyBundleAudienceExclusions,
		query.BundleID,
	}
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
		err := r.getDB(ctx).Raw(sql, smartTargetingPopulationArgs(query)...).Scan(&capacity).Error
		return &capacity, err
	}
	sql := smartTargetingPopulationCTE + `
SELECT (SELECT COUNT(*) FROM tagged_population) AS raw_unique_count,
       COUNT(*) FILTER (WHERE (?::boolean OR score_class = ANY(?::text[]))) AS eligible_unique_count
FROM classified`
	args := append(smartTargetingPopulationArgs(query),
		smartTargetingAllClasses(query.ScoreClasses), pq.Array(query.ScoreClasses))
	err := r.getDB(ctx).Raw(sql, args...).Scan(&capacity).Error
	return &capacity, err
}

func (r *smartTargetingAudienceRepository) SelectCandidates(ctx context.Context, query SmartTargetingAudienceQuery, limit int64) ([]*models.AudienceProfile, error) {
	if query.BundleID == 0 || len(query.TagIDs) == 0 || len(query.ScoreClasses) == 0 || limit <= 0 {
		return nil, fmt.Errorf("invalid smart-targeting audience selection query")
	}
	var rows []*models.AudienceProfile
	if smartTargetingAllClasses(query.ScoreClasses) {
		sql := smartTargetingSelectionBasePopulationCTE + `
SELECT id, uid, phone_number, tags, normalized_score
FROM candidate_population
ORDER BY normalized_score DESC NULLS LAST, id ASC
LIMIT ?`
		args := append(smartTargetingPopulationArgs(query), limit)
		err := r.getDB(ctx).Raw(sql, args...).Scan(&rows).Error
		return rows, err
	}
	sql := smartTargetingSelectionPopulationCTE + `
SELECT id, uid, phone_number, tags, normalized_score
FROM classified
WHERE (?::boolean OR score_class = ANY(?::text[]))
ORDER BY normalized_score DESC NULLS LAST, id ASC
LIMIT ?`
	args := append(smartTargetingPopulationArgs(query),
		smartTargetingAllClasses(query.ScoreClasses), pq.Array(query.ScoreClasses), limit)
	err := r.getDB(ctx).Raw(sql, args...).Scan(&rows).Error
	return rows, err
}

// CalculateScoreBounds computes the selected-tag union's exact p33/p66 once
// per sampling operation. The former per-tag query rebuilt and sorted this
// union for every selected tag.
func (r *smartTargetingAudienceRepository) CalculateScoreBounds(ctx context.Context, query SmartTargetingAudienceQuery) (*SmartTargetingScoreBounds, error) {
	if query.BundleID == 0 || len(query.TagIDs) == 0 || len(query.ScoreClasses) == 0 {
		return nil, fmt.Errorf("invalid smart-targeting score-bound query")
	}
	if smartTargetingAllClasses(query.ScoreClasses) {
		return &SmartTargetingScoreBounds{}, nil
	}
	sql, args := smartTargetingScoreBoundsQuery(query)
	var bounds SmartTargetingScoreBounds
	if err := r.getDB(ctx).Raw(sql, args...).Scan(&bounds).Error; err != nil {
		return nil, err
	}
	return &bounds, nil
}

const smartTargetingSampleOversamplingFactor int64 = 2

// SelectIDsForTag is the lightweight Test-preview path. Preview logic needs
// only IDs to enforce cross-tag exclusion; profile delivery columns are loaded
// only by the final campaign scheduler.
func (r *smartTargetingAudienceRepository) SelectIDsForTag(ctx context.Context, query SmartTargetingAudienceQuery, bounds *SmartTargetingScoreBounds, tagID int64, excludeAudienceIDs []int64, limit int64) ([]int64, error) {
	rows, err := r.selectSampleForTag(ctx, query, bounds, tagID, excludeAudienceIDs, limit, true)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	return ids, nil
}

// SelectForTag is the final Smart Targeting Test scheduler path. The shared
// sampler reads a bounded window from a persisted random order and shuffles
// only that window; score classes remain eligibility filters rather than row
// priority.
func (r *smartTargetingAudienceRepository) SelectForTag(ctx context.Context, query SmartTargetingAudienceQuery, bounds *SmartTargetingScoreBounds, tagID int64, excludeAudienceIDs []int64, limit int64) ([]*models.AudienceProfile, error) {
	return r.selectSampleForTag(ctx, query, bounds, tagID, excludeAudienceIDs, limit, false)
}

func smartTargetingSamplePoolLimit(limit int64) int64 {
	if limit <= 0 {
		return limit
	}
	if limit > math.MaxInt64/smartTargetingSampleOversamplingFactor {
		return math.MaxInt64
	}
	return limit * smartTargetingSampleOversamplingFactor
}

// selectSampleForTag starts at a random point in a persistent random ordering,
// reads small index-seekable pages, and shuffles a bounded oversampled result
// in memory. Cross-tag exclusions are applied in Go: sending an ever-growing
// ID array to PostgreSQL can produce a hash anti-join that destroys index order
// and reintroduces a population-wide top-N sort for later tags.
func (r *smartTargetingAudienceRepository) selectSampleForTag(ctx context.Context, query SmartTargetingAudienceQuery, bounds *SmartTargetingScoreBounds, tagID int64, excludeAudienceIDs []int64, limit int64, idsOnly bool) ([]*models.AudienceProfile, error) {
	pivot, err := smartTargetingSamplePivot(query.SamplingSeed, tagID)
	if err != nil {
		return nil, fmt.Errorf("generate smart-targeting sample pivot: %w", err)
	}
	poolLimit := smartTargetingSamplePoolLimit(limit)
	excluded := make(map[int64]struct{}, len(excludeAudienceIDs))
	for _, audienceID := range excludeAudienceIDs {
		excluded[audienceID] = struct{}{}
	}

	pool := make([]smartTargetingSampleRow, 0)
	if err := r.scanSmartTargetingSampleSegment(ctx, query, bounds, tagID, poolLimit, idsOnly, pivot, false, excluded, &pool); err != nil {
		return nil, err
	}
	if int64(len(pool)) < poolLimit {
		if err := r.scanSmartTargetingSampleSegment(ctx, query, bounds, tagID, poolLimit, idsOnly, pivot, true, excluded, &pool); err != nil {
			return nil, err
		}
	}

	rows := make([]*models.AudienceProfile, len(pool))
	for i := range pool {
		row := &pool[i]
		rows[i] = &models.AudienceProfile{
			ID:              row.ID,
			UID:             row.UID,
			PhoneNumber:     row.PhoneNumber,
			Tags:            row.Tags,
			NormalizedScore: row.NormalizedScore,
		}
	}

	shuffleSmartTargetingSample(rows, pivot)
	if int64(len(rows)) > limit {
		rows = rows[:int(limit)]
	}
	return rows, nil
}

// scanSmartTargetingSampleSegment walks one side of the circular sample order.
// Each page is capped at poolLimit and resumes strictly after its last (key,id)
// tuple. If exclusions consume a page, the next page continues without sorting
// or rescanning previously visited index entries.
func (r *smartTargetingAudienceRepository) scanSmartTargetingSampleSegment(ctx context.Context, query SmartTargetingAudienceQuery, bounds *SmartTargetingScoreBounds, tagID, poolLimit int64, idsOnly bool, pivot int64, beforePivot bool, excluded map[int64]struct{}, pool *[]smartTargetingSampleRow) error {
	var cursor *smartTargetingSampleCursor
	for int64(len(*pool)) < poolLimit {
		sql, args, err := smartTargetingPerTagSelectionQuery(query, bounds, tagID, poolLimit, idsOnly, pivot, beforePivot, cursor)
		if err != nil {
			return err
		}
		page := make([]smartTargetingSampleRow, 0)
		if err := r.getDB(ctx).Raw(sql, args...).Scan(&page).Error; err != nil {
			return err
		}
		for _, row := range page {
			if _, skip := excluded[row.ID]; skip {
				continue
			}
			*pool = append(*pool, row)
			if int64(len(*pool)) == poolLimit {
				return nil
			}
		}
		if int64(len(page)) < poolLimit {
			return nil
		}
		last := page[len(page)-1]
		cursor = &smartTargetingSampleCursor{key: last.SampleKey, id: last.ID}
	}
	return nil
}

func smartTargetingSamplePivot(seed string, tagID int64) (int64, error) {
	if seed == "" || tagID <= 0 {
		return 0, fmt.Errorf("smart-targeting sampling seed and tag are required")
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(seed))
	var rawTagID [8]byte
	binary.LittleEndian.PutUint64(rawTagID[:], uint64(tagID))
	_, _ = hash.Write(rawTagID[:])
	return int64(binary.LittleEndian.Uint64(hash.Sum(nil)[:8])), nil
}

func shuffleSmartTargetingSample(rows []*models.AudienceProfile, pivot int64) {
	if len(rows) < 2 {
		return
	}
	rng := mathrand.New(mathrand.NewSource(pivot))
	rng.Shuffle(len(rows), func(i, j int) {
		rows[i], rows[j] = rows[j], rows[i]
	})
}

func smartTargetingScoreBoundsQuery(query SmartTargetingAudienceQuery) (string, []any) {
	sql := `
	SELECT percentile_values[1] AS p33, percentile_values[2] AS p66
FROM (
    SELECT percentile_disc(ARRAY[0.33, 0.66]::double precision[])
               WITHIN GROUP (ORDER BY ap.normalized_score) AS percentile_values
    FROM audience_profiles AS ap
    WHERE ap.tags && ?::integer[]
      AND ap.phone_number IS NOT NULL
      AND BTRIM(ap.phone_number) <> ''
      AND ap.normalized_score IS NOT NULL`
	args := []any{pq.Array(query.TagIDs)}
	if len(query.AllowedColors) > 0 {
		sql += `
      AND ap.color = ANY(?::text[])`
		args = append(args, pq.Array(query.AllowedColors))
	}
	sql += `
      AND NOT EXISTS (
          SELECT 1
          FROM bundle_audience_selection_members AS used
          WHERE used.bundle_id = ? AND used.audience_id = ap.id
      )`
	args = append(args, query.BundleID)
	if query.ApplyBundleAudienceExclusions {
		sql += `
      AND NOT EXISTS (
          SELECT 1
          FROM bundle_audience_exclusions AS bundle_exclusion
          WHERE bundle_exclusion.bundle_id = ?
            AND bundle_exclusion.audience_id = ap.id
      )`
		args = append(args, query.BundleID)
	}
	sql += `
) AS calculated_bounds`
	return sql, args
}

func smartTargetingPerTagSelectionQuery(query SmartTargetingAudienceQuery, bounds *SmartTargetingScoreBounds, tagID, limit int64, idsOnly bool, pivot int64, beforePivot bool, cursor *smartTargetingSampleCursor) (string, []any, error) {
	if query.BundleID == 0 || len(query.TagIDs) == 0 || len(query.ScoreClasses) == 0 || tagID <= 0 || tagID > math.MaxInt32 || limit <= 0 {
		return "", nil, fmt.Errorf("invalid smart-targeting per-tag sample query")
	}
	selected := false
	for _, candidateTagID := range query.TagIDs {
		if candidateTagID == tagID {
			selected = true
			break
		}
	}
	if !selected {
		return "", nil, fmt.Errorf("smart-targeting per-tag sample is not in the selected tag set")
	}
	columns := "ap.id, ap.uid, ap.phone_number, ap.tags, ap.normalized_score, hashint8extended(ap.id, 0) AS sample_key"
	if idsOnly {
		columns = "ap.id, hashint8extended(ap.id, 0) AS sample_key"
	}
	sql := `
SELECT ` + columns + `
FROM audience_profiles AS ap`
	args := make([]any, 0, 10)
	if cursor == nil {
		keyComparison := ">="
		if beforePivot {
			keyComparison = "<"
		}
		sql += `
WHERE hashint8extended(ap.id, 0) ` + keyComparison + ` ?::bigint`
		args = append(args, pivot)
	} else {
		sql += `
WHERE (hashint8extended(ap.id, 0), ap.id) > (?::bigint, ?::bigint)`
		args = append(args, cursor.key, cursor.id)
		if beforePivot {
			sql += `
  AND hashint8extended(ap.id, 0) < ?::bigint`
			args = append(args, pivot)
		}
	}
	// Keep the validated int32 tag as a numeric SQL literal. pgx caches prepared
	// statements, and a generic plan with a bound tag cannot account for the
	// extreme tag-frequency skew. A tag-specific statement lets common tags use
	// the ordered sampling index and rare tags use the GIN index. No caller text
	// is interpolated.
	sql += `
  AND ap.tags @> ARRAY[` + strconv.FormatInt(tagID, 10) + `]::integer[]
  AND ap.phone_number IS NOT NULL
  AND BTRIM(ap.phone_number) <> ''`
	if len(query.AllowedColors) > 0 {
		sql += `
  AND ap.color = ANY(?::text[])`
		args = append(args, pq.Array(query.AllowedColors))
	}
	// OFFSET 0 is an intentional optimizer barrier. Without it PostgreSQL may
	// turn either correlated lookup into a hash anti-join, discard the sampling
	// index order, and sort the full eligible population before applying LIMIT.
	sql += `
  AND NOT EXISTS (
      SELECT 1
      FROM bundle_audience_selection_members AS used
      WHERE used.bundle_id = ? AND used.audience_id = ap.id
      OFFSET 0
  )`
	args = append(args, query.BundleID)
	if query.ApplyBundleAudienceExclusions {
		sql += `
  AND NOT EXISTS (
      SELECT 1
      FROM bundle_audience_exclusions AS bundle_exclusion
      WHERE bundle_exclusion.bundle_id = ?
        AND bundle_exclusion.audience_id = ap.id
      OFFSET 0
  )`
		args = append(args, query.BundleID)
	}

	scorePredicate, scoreArgs, err := smartTargetingPerTagScorePredicate(query.ScoreClasses, bounds)
	if err != nil {
		return "", nil, err
	}
	sql += scorePredicate + `
ORDER BY hashint8extended(ap.id, 0) ASC, ap.id ASC
LIMIT ?`
	args = append(args, scoreArgs...)
	args = append(args, limit)
	return sql, args, nil
}

func smartTargetingPerTagScorePredicate(classes []string, bounds *SmartTargetingScoreBounds) (string, []any, error) {
	key := strings.Join(classes, ",")
	if key == "A,B,C" {
		return "", nil, nil
	}
	if bounds == nil {
		return "", nil, fmt.Errorf("smart-targeting score bounds are required")
	}
	if (bounds.P33 == nil) != (bounds.P66 == nil) {
		return "", nil, fmt.Errorf("smart-targeting score bounds are inconsistent")
	}
	if bounds.P33 == nil {
		return "\n  AND FALSE", nil, nil
	}
	if *bounds.P33 > *bounds.P66 {
		return "", nil, fmt.Errorf("smart-targeting score bounds are invalid")
	}
	p33, p66 := *bounds.P33, *bounds.P66
	switch key {
	case "A":
		return "\n  AND ap.normalized_score > ?::double precision", []any{p66}, nil
	case "B":
		return "\n  AND ap.normalized_score > ?::double precision AND ap.normalized_score <= ?::double precision", []any{p33, p66}, nil
	case "C":
		return "\n  AND ap.normalized_score <= ?::double precision", []any{p33}, nil
	case "A,B":
		return "\n  AND ap.normalized_score > ?::double precision", []any{p33}, nil
	case "A,C":
		return "\n  AND (ap.normalized_score <= ?::double precision OR ap.normalized_score > ?::double precision)", []any{p33, p66}, nil
	case "B,C":
		return "\n  AND ap.normalized_score <= ?::double precision", []any{p66}, nil
	default:
		return "", nil, fmt.Errorf("invalid smart-targeting score classes")
	}
}
