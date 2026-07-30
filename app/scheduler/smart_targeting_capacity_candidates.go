package scheduler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"gorm.io/gorm"
)

const schedulerSmartTargetingCapacityVersion = 2

// selectAndReserveExactSmartTargetingCandidates performs selection only after
// the scheduler has claimed a campaign for execution. Capacity generations
// contain counts, never preselected audience rows.
func selectAndReserveExactSmartTargetingCandidates(
	ctx context.Context,
	db *gorm.DB,
	campaign dto.BotGetCampaignResponse,
	requested int64,
	correlationID string,
) ([]string, []int64, []string, uint, error) {
	if db == nil || campaign.BundleID == nil || *campaign.BundleID == 0 || requested <= 0 {
		return nil, nil, nil, 0, fmt.Errorf("current exact Smart Targeting capacity is unavailable for campaign %d", campaign.ID)
	}

	var phones []string
	var ids []int64
	var uids []string
	var selectionID uint
	err := repository.WithTransaction(ctx, db, func(txCtx context.Context) error {
		txDB, ok := txCtx.Value(repository.TxContextKey).(*gorm.DB)
		if !ok || txDB == nil {
			return fmt.Errorf("transaction is unavailable for campaign %d", campaign.ID)
		}
		if err := repository.LockBundleForUpdate(txCtx, *campaign.BundleID); err != nil {
			return err
		}

		selectionRepo := repository.NewBundleAudienceSelectionRepository(txDB)
		existing, err := selectionRepo.ByCampaignID(txCtx, campaign.ID)
		if err != nil {
			return err
		}
		if existing != nil {
			phones, ids, uids, err = loadReservedBundleAudience(txCtx, repository.NewAudienceProfileRepository(txDB), []int64(existing.SelectedAudienceIDs))
			if err != nil {
				return err
			}
			if err := requireExactAudienceCount(campaign.ID, requested, len(ids)); err != nil {
				return err
			}
			selectionID = existing.ID
			return nil
		}

		classes, err := normalizeSchedulerScoreClasses(campaign.AudienceGrades)
		if err != nil {
			return err
		}
		_, rawTagIDs, err := parseCampaignTagIDs(campaign)
		if err != nil {
			return err
		}
		tagIDs := make([]int64, len(rawTagIDs))
		for i, id := range rawTagIDs {
			tagIDs[i] = int64(id)
		}
		calculationRepo := repository.NewCampaignTargetingCapacityRepository(txDB)
		calculation, err := calculationRepo.CurrentForExecution(txCtx, campaign.ID, *campaign.BundleID, tagIDs, classes, schedulerSmartTargetingCapacityVersion, time.Now().UTC())
		if err != nil {
			return err
		}
		if calculation == nil || calculation.UsableUniqueAudienceCount < requested {
			return fmt.Errorf("current exact Smart Targeting capacity is unavailable for campaign %d", campaign.ID)
		}

		// fingerprint, err := schedulerApprovedAllocationFingerprint(txCtx, txDB, calculation.BundleID, campaign.ID)
		// if err != nil {
		// 	return err
		// }
		// if fingerprint != calculation.AllocationFingerprint {
		// 	return fmt.Errorf("exact Smart Targeting capacity is stale for campaign %d", campaign.ID)
		// }

		// The approval fingerprint is deliberately not an execution precondition.
		// Other campaigns naturally change from approved to running/materialized
		// after this campaign was approved, which changes that fingerprint without
		// invalidating this campaign's reservation. Under the Bundle lock, the
		// candidate query below is the authoritative current population and the
		// exact-count check fails safely if capacity has genuinely disappeared.
		rows, err := repository.NewSmartTargetingAudienceRepository(txDB).SelectCandidates(txCtx, repository.SmartTargetingAudienceQuery{
			BundleID: *campaign.BundleID, TagIDs: tagIDs, ScoreClasses: classes,
		}, requested)
		if err != nil {
			return err
		}
		phones, ids, uids = audienceProfileRows(rows)
		if err := requireExactAudienceCount(campaign.ID, requested, len(ids)); err != nil {
			return err
		}
		selection, err := selectionRepo.InsertForCampaign(txCtx, campaign.CustomerID, *campaign.BundleID, campaign.ID, correlationID, ids)
		if err != nil {
			return err
		}
		if selection == nil || selection.ID == 0 {
			return fmt.Errorf("bundle audience selection was not persisted for campaign %d", campaign.ID)
		}
		selectionID = selection.ID
		return nil
	})
	if err != nil {
		return nil, nil, nil, 0, err
	}
	return phones, ids, uids, selectionID, nil
}

func audienceProfileRows(rows []*models.AudienceProfile) ([]string, []int64, []string) {
	phones := make([]string, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	uids := make([]string, 0, len(rows))
	for _, row := range rows {
		if row == nil || row.PhoneNumber == nil || strings.TrimSpace(*row.PhoneNumber) == "" {
			continue
		}
		phones = append(phones, strings.TrimSpace(*row.PhoneNumber))
		ids = append(ids, int64(row.ID))
		uids = append(uids, row.UID)
	}
	return phones, ids, uids
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
		if _, exists := seen[class]; exists {
			return nil, fmt.Errorf("duplicate audience score class for campaign execution")
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

// func schedulerApprovedAllocationFingerprint(ctx context.Context, db *gorm.DB, bundleID, currentCampaignID uint) (string, error) {
// 	rows, err := repository.ListBundleCampaignAllocations(ctx, db, bundleID, currentCampaignID)
// 	if err != nil {
// 		return "", err
// 	}
// 	parts := make([]string, 0, len(rows))
// 	for _, row := range rows {
// 		if row.NumAudience == nil {
// 			return "", fmt.Errorf("reserved campaign %d has no audience allocation", row.CampaignID)
// 		}
// 		amount := *row.NumAudience
// 		if amount > uint64(math.MaxInt64) {
// 			return "", fmt.Errorf("approved campaign audience allocation overflows bigint")
// 		}
// 		parts = append(parts, strconv.FormatUint(uint64(row.CampaignID), 10)+":"+strconv.FormatUint(amount, 10)+":"+string(row.Status)+":"+strconv.FormatBool(row.Materialized))
// 	}
// 	fingerprintInput := "v3|bundle=" + strconv.FormatUint(uint64(bundleID), 10) + "|allocations=" + strings.Join(parts, ",")
// 	sum := sha256.Sum256([]byte(fingerprintInput))
// 	return hex.EncodeToString(sum[:]), nil
// }
