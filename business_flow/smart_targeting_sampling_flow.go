package businessflow

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/lib/pq"
	"gorm.io/gorm/clause"
)

type smartTargetingTestSample struct {
	order       []uint
	satisfied   []dto.SmartTargetingTestSamplingTagResult
	unsatisfied []dto.SmartTargetingTestSamplingTagResult
	effective   uint64
	inputHash   string
}

type smartTargetingTestSamplingInput struct {
	order   []uint
	classes []string
	hash    string
}

type smartTargetingTestSamplingIntent struct {
	satisfied []uint
	effective uint64
}

func checkedSmartTargetingTestAudienceCount(satisfiedTagCount int, sampleSizePerTag uint64) (uint64, error) {
	if satisfiedTagCount < 0 || sampleSizePerTag == 0 || uint64(satisfiedTagCount) > uint64(math.MaxInt64)/sampleSizePerTag {
		return 0, ErrSmartTargetingTestAudienceCountOverflow
	}
	return uint64(satisfiedTagCount) * sampleSizePerTag, nil
}

func smartTargetingTestSamplingHash(campaign *models.Campaign, orderedTagIDs []uint, classes []string) (string, error) {
	if campaign == nil || campaign.BundleID == nil || campaign.SampleSizePerTag == nil {
		return "", ErrSmartTargetingTestPreviewRequired
	}
	parts := make([]string, len(orderedTagIDs))
	for i, id := range orderedTagIDs {
		parts[i] = strconv.FormatUint(uint64(id), 10)
	}
	value := "feature4-v1|campaign=" + strconv.FormatUint(uint64(campaign.ID), 10) +
		"|bundle=" + strconv.FormatUint(uint64(*campaign.BundleID), 10) +
		"|sample=" + strconv.FormatUint(*campaign.SampleSizePerTag, 10) +
		"|tags=" + strings.Join(parts, ",") +
		"|classes=" + strings.Join(classes, ",")
	return hashSmartTargetingCapacityString(value), nil
}

func currentSmartTargetingTestSamplingInput(ctx context.Context, selectedTagRepo repository.CampaignSelectedTagRepository, campaign *models.Campaign) (*smartTargetingTestSamplingInput, error) {
	if campaign == nil || !campaign.Spec.UsesSmartTargeting() || campaign.Phase != models.CampaignPhaseTest {
		return nil, ErrSmartTargetingTestPreviewRequired
	}
	if campaign.BundleID == nil || *campaign.BundleID == 0 {
		return nil, ErrBundleNotFound
	}
	if campaign.SampleSizePerTag == nil {
		return nil, ErrSmartTargetingSampleSizeRequired
	}
	if *campaign.SampleSizePerTag == 0 || *campaign.SampleSizePerTag > math.MaxInt64 {
		return nil, ErrSmartTargetingSampleSizeInvalid
	}
	selected, err := selectedTagRepo.ListSelected(ctx, campaign.ID)
	if err != nil {
		return nil, err
	}
	if len(selected) == 0 {
		return nil, ErrSmartTargetingTagsRequired
	}
	input := &smartTargetingTestSamplingInput{order: make([]uint, 0, len(selected))}
	for _, selectedTag := range selected {
		if selectedTag == nil || selectedTag.TagID == 0 {
			return nil, ErrSmartTargetingTagInvalid
		}
		input.order = append(input.order, selectedTag.TagID)
	}
	input.classes, err = normalizeSmartTargetingScoreClasses(campaign.Spec.AudienceGrades)
	if err != nil {
		return nil, err
	}
	input.hash, err = smartTargetingTestSamplingHash(campaign, input.order, input.classes)
	if err != nil {
		return nil, err
	}
	return input, nil
}

func currentSmartTargetingTestSamplingIntent(ctx context.Context, selectedTagRepo repository.CampaignSelectedTagRepository, campaign *models.Campaign, requireSatisfied bool) (*smartTargetingTestSamplingIntent, error) {
	input, err := currentSmartTargetingTestSamplingInput(ctx, selectedTagRepo, campaign)
	if err != nil {
		return nil, err
	}
	if campaign.SmartTargetingTestSamplingInputHash == nil || campaign.SmartTargetingTestSamplingPreviewedAt == nil || *campaign.SmartTargetingTestSamplingInputHash != input.hash {
		return nil, ErrSmartTargetingTestPreviewRequired
	}
	satisfied := make([]uint, 0, len(campaign.SmartTargetingTestSatisfiedTagIDs))
	selectedPosition := 0
	seen := make(map[uint]struct{}, len(campaign.SmartTargetingTestSatisfiedTagIDs))
	for _, rawID := range campaign.SmartTargetingTestSatisfiedTagIDs {
		if rawID <= 0 || uint64(rawID) > uint64(^uint(0)) {
			return nil, ErrSmartTargetingTestPreviewRequired
		}
		id := uint(rawID)
		if _, exists := seen[id]; exists {
			return nil, ErrSmartTargetingTestPreviewRequired
		}
		seen[id] = struct{}{}
		for selectedPosition < len(input.order) && input.order[selectedPosition] != id {
			selectedPosition++
		}
		if selectedPosition == len(input.order) {
			return nil, ErrSmartTargetingTestPreviewRequired
		}
		selectedPosition++
		satisfied = append(satisfied, id)
	}
	if requireSatisfied && len(satisfied) == 0 {
		return nil, ErrSmartTargetingTestNoSatisfiedTags
	}
	effective, err := checkedSmartTargetingTestAudienceCount(len(satisfied), *campaign.SampleSizePerTag)
	if err != nil {
		return nil, err
	}
	return &smartTargetingTestSamplingIntent{
		satisfied: satisfied,
		effective: effective,
	}, nil
}

func (s *CampaignFlowImpl) calculateSmartTargetingTestSample(ctx context.Context, campaign *models.Campaign) (*smartTargetingTestSample, error) {
	if campaign == nil || !campaign.Spec.UsesSmartTargeting() || campaign.Phase != models.CampaignPhaseTest {
		return nil, fmt.Errorf("Smart Targeting Test campaign is required")
	}
	if campaign.BundleID == nil || *campaign.BundleID == 0 {
		return nil, ErrBundleNotFound
	}
	if campaign.SampleSizePerTag == nil {
		return nil, ErrSmartTargetingSampleSizeRequired
	}
	if *campaign.SampleSizePerTag == 0 || *campaign.SampleSizePerTag > math.MaxInt64 {
		return nil, ErrSmartTargetingSampleSizeInvalid
	}
	input, err := currentSmartTargetingTestSamplingInput(ctx, s.selectedTagRepo, campaign)
	if err != nil {
		return nil, err
	}
	tagIDs := make([]int64, 0, len(input.order))
	result := &smartTargetingTestSample{
		order:       append([]uint(nil), input.order...),
		satisfied:   make([]dto.SmartTargetingTestSamplingTagResult, 0, len(input.order)),
		unsatisfied: make([]dto.SmartTargetingTestSamplingTagResult, 0),
		inputHash:   input.hash,
	}
	for _, tagID := range input.order {
		tagIDs = append(tagIDs, int64(tagID))
	}

	// TODO(feature-4-approved-reservations): approved campaigns have no concrete
	// audience IDs until their schedulers run. A preview cannot exclude those
	// future allocations; the scheduler remains the availability source of truth.
	excluded := make([]int64, 0)
	audienceRepo := repository.NewSmartTargetingAudienceRepository(s.db)
	query := repository.SmartTargetingAudienceQuery{BundleID: *campaign.BundleID, TagIDs: tagIDs, ScoreClasses: input.classes}
	for position, tagID := range tagIDs {
		rows, err := audienceRepo.SelectRandomForTag(ctx, query, tagID, excluded, int64(*campaign.SampleSizePerTag))
		if err != nil {
			return nil, err
		}
		item := dto.SmartTargetingTestSamplingTagResult{
			TagID: uint(tagID), SelectionOrder: position, AvailableCount: int64(len(rows)),
			Satisfied: uint64(len(rows)) == *campaign.SampleSizePerTag,
		}
		if !item.Satisfied {
			result.unsatisfied = append(result.unsatisfied, item)
			continue
		}
		result.satisfied = append(result.satisfied, item)
		for _, row := range rows {
			excluded = append(excluded, row.ID)
		}
	}
	result.effective, err = checkedSmartTargetingTestAudienceCount(len(result.satisfied), *campaign.SampleSizePerTag)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func checkedCampaignCost(pricePerMessage, audienceCount uint64) (uint64, error) {
	if audienceCount != 0 && pricePerMessage > math.MaxUint64/audienceCount {
		return 0, NewBusinessError("CAMPAIGN_COST_OVERFLOW", "Campaign cost exceeds the supported range", ErrCampaignCostOverflow)
	}
	return pricePerMessage * audienceCount, nil
}

func clearCampaignSmartTargetingTestSamplingPreviewFields(campaign *models.Campaign) {
	if campaign == nil {
		return
	}
	campaign.SmartTargetingTestSatisfiedTagIDs = pq.Int64Array{}
	campaign.SmartTargetingTestSamplingInputHash = nil
	campaign.SmartTargetingTestSamplingPreviewedAt = nil
	campaign.NumAudience = utils.ToPtr(uint64(0))
}

func (s *CampaignFlowImpl) clearCampaignSmartTargetingTestSamplingPreview(ctx context.Context, campaignID uint) error {
	return smartTargetingDB(ctx, s.db).Model(&models.Campaign{}).Where("id = ?", campaignID).Updates(map[string]any{
		"smart_targeting_test_satisfied_tag_ids":     pq.Int64Array{},
		"smart_targeting_test_sampling_input_hash":   nil,
		"smart_targeting_test_sampling_previewed_at": nil,
		"num_audience": uint64(0),
		"updated_at":   utils.UTCNow(),
	}).Error
}

func (s *CampaignFlowImpl) persistSmartTargetingTestSamplingPreview(ctx context.Context, campaignID uint, expectedRevision *time.Time, sample *smartTargetingTestSample) error {
	if sample == nil {
		return ErrSmartTargetingTestPreviewRequired
	}
	return repository.WithTransaction(ctx, s.db, func(txCtx context.Context) error {
		txDB := smartTargetingDB(txCtx, s.db)
		var campaign models.Campaign
		if err := txDB.Clauses(clause.Locking{Strength: "UPDATE"}).First(&campaign, campaignID).Error; err != nil {
			return err
		}
		if !campaign.IsEditable() {
			return ErrCampaignUpdateNotAllowed
		}
		if (expectedRevision == nil) != (campaign.UpdatedAt == nil) ||
			(expectedRevision != nil && !campaign.UpdatedAt.Equal(*expectedRevision)) {
			return ErrSmartTargetingTestPreviewRequired
		}
		input, err := currentSmartTargetingTestSamplingInput(txCtx, s.selectedTagRepo, &campaign)
		if err != nil {
			return err
		}
		if input.hash != sample.inputHash {
			return ErrSmartTargetingTestPreviewRequired
		}
		satisfied := make(pq.Int64Array, 0, len(sample.satisfied))
		for _, item := range sample.satisfied {
			satisfied = append(satisfied, int64(item.TagID))
		}
		now := time.Now().UTC()
		return txDB.Model(&models.Campaign{}).Where("id = ?", campaign.ID).Updates(map[string]any{
			"smart_targeting_test_satisfied_tag_ids":     satisfied,
			"smart_targeting_test_sampling_input_hash":   sample.inputHash,
			"smart_targeting_test_sampling_previewed_at": now,
			"num_audience": sample.effective,
			"updated_at":   utils.UTCNow(),
		}).Error
	})
}

func (s *CampaignFlowImpl) PreviewSmartTargetingTestSampling(ctx context.Context, req *dto.SmartTargetingTestSamplingPreviewRequest, metadata *ClientMetadata) (*dto.SmartTargetingTestSamplingPreviewResponse, error) {
	if req == nil || req.CustomerID == 0 || req.CampaignUUID == "" {
		return nil, NewBusinessError("SMART_TARGETING_TEST_PREVIEW_INVALID", "Invalid Smart Targeting Test preview request", ErrCampaignNotFound)
	}
	campaign, err := getCampaign(ctx, s.campaignRepo, req.CampaignUUID, req.CustomerID)
	if err != nil {
		return nil, NewBusinessError("CAMPAIGN_LOOKUP_FAILED", "Failed to lookup campaign", err)
	}
	if !campaign.Spec.UsesSmartTargeting() || campaign.Phase != models.CampaignPhaseTest {
		return nil, NewBusinessError("SMART_TARGETING_TEST_PREVIEW_INVALID", "Smart Targeting Test campaign is required", ErrInvalidState)
	}
	if !campaign.IsEditable() {
		return nil, NewBusinessError("CAMPAIGN_UPDATE_NOT_ALLOWED", "Sampling preview cannot change a finalized campaign", ErrCampaignUpdateNotAllowed)
	}
	if _, err := CurrentSmartTargetingCapacity(ctx, s.db, s.selectedTagRepo, s.capacityCalculationRepo, &campaign); err != nil {
		return nil, NewBusinessError("SMART_TARGETING_EXACT_CAPACITY_REQUIRED", "A current exact Smart Targeting capacity calculation is required", err)
	}
	sample, err := s.calculateSmartTargetingTestSample(ctx, &campaign)
	if err != nil {
		return nil, NewBusinessError("SMART_TARGETING_TEST_PREVIEW_FAILED", "Failed to calculate Smart Targeting Test sample", err)
	}
	pricePerMessage, _, err := s.computeCostInputs(ctx, campaign, metadata)
	if err != nil {
		return nil, err
	}
	cost, err := checkedCampaignCost(pricePerMessage, sample.effective)
	if err != nil {
		return nil, err
	}
	if customer, lookupErr := getCustomer(ctx, s.customerRepo, req.CustomerID); lookupErr == nil && s.adminConfig.HasMobile(customer.RepresentativeMobile) {
		cost = 0
	}
	if err := s.persistSmartTargetingTestSamplingPreview(ctx, campaign.ID, campaign.UpdatedAt, sample); err != nil {
		return nil, NewBusinessError("SMART_TARGETING_TEST_PREVIEW_PERSIST_FAILED", "Failed to persist Smart Targeting Test sampling intent", err)
	}
	return &dto.SmartTargetingTestSamplingPreviewResponse{
		SampleSizePerTag:       *campaign.SampleSizePerTag,
		TagSamplingOrder:       sample.order,
		SatisfiedTags:          sample.satisfied,
		UnsatisfiedTags:        sample.unsatisfied,
		SatisfiedTagCount:      len(sample.satisfied),
		EffectiveAudienceCount: sample.effective,
		CampaignCost:           cost,
	}, nil
}
