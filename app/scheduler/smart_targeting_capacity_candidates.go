package scheduler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

// selectAndReserveExactSmartTargetingCandidates validates the current
// generation, chooses candidates, and appends the Bundle usage snapshot while
// holding the same Bundle lock used by approvals and other audience selectors.
func selectAndReserveExactSmartTargetingCandidates(
	ctx context.Context,
	db *gorm.DB,
	campaign dto.BotGetCampaignResponse,
	requested int64,
	preferWhite bool,
	correlationID string,
) ([]string, []int64, []string, uint, error) {
	if db == nil || campaign.BundleID == nil || *campaign.BundleID == 0 {
		return nil, nil, nil, 0, fmt.Errorf("current exact Smart Targeting capacity is unavailable for campaign %d", campaign.ID)
	}
	var phones []string
	var ids []int64
	var uids []string
	var selection *models.BundleAudienceSelection
	err := repository.WithTransaction(ctx, db, func(txCtx context.Context) error {
		txDB := db
		if tx, ok := txCtx.Value(repository.TxContextKey).(*gorm.DB); ok && tx != nil {
			txDB = tx.WithContext(txCtx)
		}
		if err := txDB.Exec("SELECT id FROM bundles WHERE id = ? FOR UPDATE", *campaign.BundleID).Error; err != nil {
			return err
		}
		selectionRepo := repository.NewBundleAudienceSelectionRepository(txDB)
		latest, err := selectionRepo.Latest(txCtx, campaign.CustomerID, *campaign.BundleID)
		if err != nil {
			return err
		}
		exclude := make(map[int64]struct{})
		if latest != nil {
			for _, id := range latest.AudienceIDs {
				exclude[id] = struct{}{}
			}
		}
		phones, ids, uids, err = selectExactSmartTargetingCandidates(txCtx, txDB, campaign, requested, exclude, preferWhite)
		if err != nil {
			return err
		}
		selection, err = selectionRepo.InsertWithMerge(txCtx, campaign.CustomerID, *campaign.BundleID, correlationID, ids)
		return err
	})
	if err != nil {
		return nil, nil, nil, 0, err
	}
	if selection == nil || selection.ID == 0 {
		return nil, nil, nil, 0, fmt.Errorf("bundle audience selection was not persisted for campaign %d", campaign.ID)
	}
	return phones, ids, uids, selection.ID, nil
}

// selectExactSmartTargetingCandidates is the execution-side counterpart of
// the capacity worker. It refuses a missing, expired, changed-tag, or
// changed-allocation generation instead of silently falling back to the old
// direct tag query, which would make an approved campaign exceed its priced
// capacity. Bundle rows selected since the calculation are still excluded as
// a final safety net.
func selectExactSmartTargetingCandidates(ctx context.Context, db *gorm.DB, campaign dto.BotGetCampaignResponse, requested int64, exclude map[int64]struct{}, preferWhite bool) ([]string, []int64, []string, error) {
	if !usesSmartAudienceTargeting(campaign) {
		return nil, nil, nil, nil
	}
	if db == nil || campaign.BundleID == nil || *campaign.BundleID == 0 || requested <= 0 {
		return nil, nil, nil, fmt.Errorf("current exact Smart Targeting capacity is unavailable for campaign %d", campaign.ID)
	}
	classes, err := normalizeSchedulerScoreClasses(campaign.AudienceGrades)
	if err != nil {
		return nil, nil, nil, err
	}

	type calculationRow struct {
		ID                    int64
		BundleID              uint
		UsableCapacity        int64  `gorm:"column:usable_unique_audience_count"`
		AllocationFingerprint string `gorm:"column:allocation_fingerprint"`
	}
	var calculation calculationRow
	query := `
SELECT c.id, c.bundle_id, c.usable_unique_audience_count, c.allocation_fingerprint
FROM campaign_targeting_capacity_calculations c
WHERE c.campaign_id = ?
  AND c.status = 'calculated'
  AND c.calculation_version = 1
  AND c.expires_at > ?
  AND c.platform = ?
  AND c.selected_score_classes = ?::text[]
  AND c.selected_tag_ids = (
      SELECT COALESCE(array_agg(cst.tag_id::bigint ORDER BY cst.tag_id), ARRAY[]::bigint[])
      FROM campaign_selected_tags cst
      WHERE cst.campaign_id = ?
  )
ORDER BY c.created_at DESC, c.id DESC
LIMIT 1`
	if err := db.WithContext(ctx).Raw(query, campaign.ID, time.Now().UTC(), strings.ToLower(strings.TrimSpace(campaign.Platform)), pq.Array(classes), campaign.ID).Scan(&calculation).Error; err != nil {
		return nil, nil, nil, err
	}
	if calculation.ID == 0 || calculation.BundleID != *campaign.BundleID || calculation.UsableCapacity < requested {
		return nil, nil, nil, fmt.Errorf("current exact Smart Targeting capacity is unavailable for campaign %d", campaign.ID)
	}

	fingerprint, err := schedulerApprovedAllocationFingerprint(ctx, db, calculation.BundleID, campaign.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	if fingerprint != calculation.AllocationFingerprint {
		return nil, nil, nil, fmt.Errorf("exact Smart Targeting capacity is stale for campaign %d", campaign.ID)
	}

	excluded := make([]int64, 0, len(exclude))
	for id := range exclude {
		excluded = append(excluded, id)
	}
	sort.Slice(excluded, func(i, j int) bool { return excluded[i] < excluded[j] })
	type candidateRow struct {
		ID          int64  `gorm:"column:id"`
		UID         string `gorm:"column:uid"`
		PhoneNumber string `gorm:"column:phone_number"`
	}
	var rows []candidateRow
	candidateQuery := `
SELECT ap.id, ap.uid, ap.phone_number
FROM campaign_targeting_candidate_stack stack
JOIN audience_profiles ap ON ap.id = stack.audience_id
WHERE stack.calculation_id = ?
  AND stack.expires_at > ?
  AND ap.phone_number IS NOT NULL
  AND BTRIM(ap.phone_number) <> ''
  AND NOT (ap.id = ANY(?::bigint[]))
ORDER BY
  CASE WHEN ?::boolean AND ap.color = 'white' THEN 0
       WHEN ?::boolean THEN 1
       ELSE 0 END,
  ap.id DESC
LIMIT ?`
	if err := db.WithContext(ctx).Raw(candidateQuery, calculation.ID, time.Now().UTC(), pq.Array(excluded), preferWhite, preferWhite, requested).Scan(&rows).Error; err != nil {
		return nil, nil, nil, err
	}
	if int64(len(rows)) != requested {
		return nil, nil, nil, fmt.Errorf("exact Smart Targeting candidates are incomplete for campaign %d: requested=%d available=%d", campaign.ID, requested, len(rows))
	}
	phones := make([]string, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	uids := make([]string, 0, len(rows))
	for _, row := range rows {
		phones = append(phones, strings.TrimSpace(row.PhoneNumber))
		ids = append(ids, row.ID)
		uids = append(uids, row.UID)
	}
	return phones, ids, uids, nil
}

func normalizeSchedulerScoreClasses(input []string) ([]string, error) {
	if len(input) == 0 {
		return []string{"A", "B", "C"}, nil
	}
	seen := make(map[string]struct{}, len(input))
	for _, raw := range input {
		class := strings.ToUpper(strings.TrimSpace(raw))
		if class != "A" && class != "B" && class != "C" {
			return nil, fmt.Errorf("invalid audience score class for campaign execution")
		}
		seen[class] = struct{}{}
	}
	classes := make([]string, 0, len(seen))
	for _, class := range []string{"A", "B", "C"} {
		if _, ok := seen[class]; ok {
			classes = append(classes, class)
		}
	}
	return classes, nil
}

func schedulerApprovedAllocationFingerprint(ctx context.Context, db *gorm.DB, bundleID, currentCampaignID uint) (string, error) {
	type row struct {
		ID          uint
		NumAudience *uint64
		Status      models.CampaignStatus
	}
	var rows []row
	if err := db.WithContext(ctx).Table("campaigns").Select("id, num_audience, status").
		Where("bundle_id = ? AND id <> ? AND status IN ?", bundleID, currentCampaignID, []models.CampaignStatus{
			models.CampaignStatusApproved, models.CampaignStatusRunning, models.CampaignStatusExecuted,
		}).
		Order("id ASC").Find(&rows).Error; err != nil {
		return "", err
	}
	parts := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.NumAudience == nil {
			return "", fmt.Errorf("reserved campaign %d has no audience allocation", row.ID)
		}
		amount := *row.NumAudience
		if amount > uint64(math.MaxInt64) {
			return "", fmt.Errorf("approved campaign audience allocation overflows bigint")
		}
		parts = append(parts, strconv.FormatUint(uint64(row.ID), 10)+":"+strconv.FormatUint(amount, 10))
	}
	fingerprintInput := "v2|bundle=" + strconv.FormatUint(uint64(bundleID), 10) + "|allocations=" + strings.Join(parts, ",")
	sum := sha256.Sum256([]byte(fingerprintInput))
	return hex.EncodeToString(sum[:]), nil
}
