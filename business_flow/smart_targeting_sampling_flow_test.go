package businessflow

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/utils"
	"github.com/lib/pq"
)

type samplingSelectedTagRepositoryStub struct {
	selected []*models.CampaignSelectedTag
}

func (s *samplingSelectedTagRepositoryStub) ListAvailable(context.Context, uint, uint, string, string, string, int, int) ([]*models.SmartTargetingTagRow, int64, error) {
	return nil, 0, nil
}

func (s *samplingSelectedTagRepositoryStub) ListAvailableTagIDs(context.Context, uint, string, string, string, int) ([]uint, error) {
	return nil, nil
}

func (s *samplingSelectedTagRepositoryStub) ListSelected(context.Context, uint) ([]*models.CampaignSelectedTag, error) {
	return s.selected, nil
}

func (s *samplingSelectedTagRepositoryStub) Summary(context.Context, uint) (*models.CampaignSelectedTagSummary, error) {
	return &models.CampaignSelectedTagSummary{SelectedTagCount: int64(len(s.selected))}, nil
}

func (s *samplingSelectedTagRepositoryStub) Validate(context.Context, uint, uint) error {
	return nil
}

func (s *samplingSelectedTagRepositoryStub) Replace(context.Context, uint, uint, uint, []uint) error {
	return nil
}

func (s *samplingSelectedTagRepositoryStub) Clear(context.Context, uint) error {
	return nil
}

func TestSmartTargetingTestCreateRequiresPositiveSampleSizePerTag(t *testing.T) {
	flow := &CampaignFlowImpl{}
	method := models.CampaignAudienceTargetingSmart
	phase := string(models.CampaignPhaseTest)
	title := "test"
	bundleID := uint(3)
	base := dto.CreateCampaignRequest{
		CustomerID: 7, Title: &title, BundleID: &bundleID, Phase: &phase,
		AudienceTargetingMethod: &method, SelectedTagIDs: []uint{9, 2},
	}
	if err := flow.validateCreateCampaignRequest(t.Context(), &base); !errors.Is(err, ErrSmartTargetingSampleSizeRequired) {
		t.Fatalf("missing sample_size_per_tag error = %v", err)
	}

	base.SampleSizePerTag = utils.ToPtr(uint64(0))
	if err := flow.validateCreateCampaignRequest(t.Context(), &base); !errors.Is(err, ErrSmartTargetingSampleSizeInvalid) {
		t.Fatalf("zero sample_size_per_tag error = %v", err)
	}
}

func TestExecutionDoesNotRequireSampleSizePerTag(t *testing.T) {
	method := models.CampaignAudienceTargetingSmart
	phase := string(models.CampaignPhaseExecution)
	title := "execution"
	bundleID := uint(3)
	platform := models.CampaignPlatformBale
	req := dto.CreateCampaignRequest{
		CustomerID: 7, Title: &title, BundleID: &bundleID, Phase: &phase, Platform: &platform,
		AudienceTargetingMethod: &method, SelectedTagIDs: []uint{9, 2},
	}
	err := (&CampaignFlowImpl{}).validateCreateCampaignRequest(t.Context(), &req)
	if errors.Is(err, ErrSmartTargetingSampleSizeRequired) || errors.Is(err, ErrSmartTargetingSampleSizeInvalid) {
		t.Fatalf("execution unexpectedly depends on sample_size_per_tag: %v", err)
	}
}

func TestSmartTargetingTestSamplingHashPreservesTagOrder(t *testing.T) {
	method := models.CampaignAudienceTargetingSmart
	bundleID := uint(3)
	sampleSize := uint64(600)
	campaign := &models.Campaign{
		ID:               17,
		BundleID:         &bundleID,
		SampleSizePerTag: &sampleSize,
		Spec:             models.CampaignSpec{AudienceTargetingMethod: &method},
	}
	first, err := smartTargetingTestSamplingHash(campaign, []uint{9, 2, 5}, []string{"A", "C"})
	if err != nil {
		t.Fatalf("first sampling hash failed: %v", err)
	}
	second, err := smartTargetingTestSamplingHash(campaign, []uint{2, 9, 5}, []string{"A", "C"})
	if err != nil {
		t.Fatalf("second sampling hash failed: %v", err)
	}
	if first == second {
		t.Fatal("sampling input hash must change when the user-defined tag order changes")
	}
}

func TestCheckedCampaignCostRejectsOverflow(t *testing.T) {
	if _, err := checkedCampaignCost(math.MaxUint64, 2); !errors.Is(err, ErrCampaignCostOverflow) {
		t.Fatalf("overflow error = %v, want ErrCampaignCostOverflow", err)
	}
	if got, err := checkedCampaignCost(13, 7); err != nil || got != 91 {
		t.Fatalf("checked cost = (%d, %v), want (91, nil)", got, err)
	}
}

func TestSmartTargetingTestFinalizationDerivesBudget(t *testing.T) {
	method := models.CampaignAudienceTargetingSmart
	campaign := &models.Campaign{
		Phase: models.CampaignPhaseTest,
		Spec:  models.CampaignSpec{AudienceTargetingMethod: &method},
	}

	if err := validateCampaignFinalizationBudget(campaign); err != nil {
		t.Fatalf("missing explicit budget error = %v, want nil", err)
	}

	audienceCount, err := checkedSmartTargetingTestAudienceCount(2, 600)
	if err != nil {
		t.Fatalf("effective audience calculation failed: %v", err)
	}
	totalCost, err := checkedCampaignCost(1_200, audienceCount)
	if err != nil {
		t.Fatalf("campaign cost calculation failed: %v", err)
	}
	cost := &dto.CalculateCampaignCostResponse{
		TotalCost:         totalCost,
		NumTargetAudience: audienceCount,
	}
	applyFinalizedCampaignCost(campaign, cost)

	if campaign.Spec.Budget == nil || *campaign.Spec.Budget != cost.TotalCost {
		t.Fatalf("derived budget = %v, want %d", campaign.Spec.Budget, cost.TotalCost)
	}
	if campaign.NumAudience == nil || *campaign.NumAudience != cost.NumTargetAudience {
		t.Fatalf("derived audience count = %v, want %d", campaign.NumAudience, cost.NumTargetAudience)
	}
}

func TestExplicitBudgetRemainsRequiredOutsideSmartTargetingTest(t *testing.T) {
	method := models.CampaignAudienceTargetingSmart
	campaign := &models.Campaign{
		Phase: models.CampaignPhaseExecution,
		Spec:  models.CampaignSpec{AudienceTargetingMethod: &method},
	}
	if err := validateCampaignFinalizationBudget(campaign); !errors.Is(err, ErrCampaignBudgetRequired) {
		t.Fatalf("missing execution budget error = %v, want ErrCampaignBudgetRequired", err)
	}

	method = models.CampaignAudienceTargetingStandard
	campaign.Phase = models.CampaignPhaseTest
	campaign.Spec.AudienceTargetingMethod = &method
	if err := validateCampaignFinalizationBudget(campaign); !errors.Is(err, ErrCampaignBudgetRequired) {
		t.Fatalf("missing standard Test budget error = %v, want ErrCampaignBudgetRequired", err)
	}
}

func TestUpdateBudgetValidationIgnoresClientDefaultsForSmartTargetingTest(t *testing.T) {
	zero := uint64(0)
	if err := validateUpdateCampaignBudget(&zero, models.CampaignAudienceTargetingSmart, models.CampaignPhaseTest); err != nil {
		t.Fatalf("Smart Targeting Test zero budget error = %v, want nil", err)
	}

	if err := validateUpdateCampaignBudget(&zero, models.CampaignAudienceTargetingSmart, models.CampaignPhaseExecution); !errors.Is(err, ErrCampaignBudgetRequired) {
		t.Fatalf("Smart Targeting Execution zero budget error = %v, want ErrCampaignBudgetRequired", err)
	}

	tooSmall := minCampaignBudget - 1
	if err := validateUpdateCampaignBudget(&tooSmall, models.CampaignAudienceTargetingStandard, models.CampaignPhaseTest); !errors.Is(err, ErrCampaignBudgetOutOfRange) {
		t.Fatalf("standard out-of-range budget error = %v, want ErrCampaignBudgetOutOfRange", err)
	}
}

func TestCheckedSmartTargetingTestAudienceCountFitsDatabaseAndSchedulerRange(t *testing.T) {
	if _, err := checkedSmartTargetingTestAudienceCount(2, math.MaxInt64); !errors.Is(err, ErrSmartTargetingTestAudienceCountOverflow) {
		t.Fatalf("audience overflow error = %v, want ErrSmartTargetingTestAudienceCountOverflow", err)
	}
	if got, err := checkedSmartTargetingTestAudienceCount(3, 600); err != nil || got != 1_800 {
		t.Fatalf("checked audience count = (%d, %v), want (1800, nil)", got, err)
	}
}

func TestCurrentSmartTargetingTestSamplingIntentValidatesOrderedSubset(t *testing.T) {
	method := models.CampaignAudienceTargetingSmart
	bundleID := uint(3)
	sampleSize := uint64(600)
	previewedAt := time.Now().UTC()
	campaign := &models.Campaign{
		ID:               17,
		BundleID:         &bundleID,
		Phase:            models.CampaignPhaseTest,
		SampleSizePerTag: &sampleSize,
		Spec: models.CampaignSpec{
			AudienceTargetingMethod: &method,
			AudienceGrades:          []string{"A", "C"},
		},
		SmartTargetingTestSatisfiedTagIDs:     pq.Int64Array{9, 5},
		SmartTargetingTestSamplingPreviewedAt: &previewedAt,
	}
	repo := &samplingSelectedTagRepositoryStub{selected: []*models.CampaignSelectedTag{
		{TagID: 9, SelectionOrder: 0},
		{TagID: 2, SelectionOrder: 1},
		{TagID: 5, SelectionOrder: 2},
	}}
	hash, err := smartTargetingTestSamplingHash(campaign, []uint{9, 2, 5}, []string{"A", "C"})
	if err != nil {
		t.Fatalf("sampling hash failed: %v", err)
	}
	campaign.SmartTargetingTestSamplingInputHash = &hash

	intent, err := currentSmartTargetingTestSamplingIntent(t.Context(), repo, campaign, true)
	if err != nil || intent.effective != 1_200 || len(intent.satisfied) != 2 || intent.satisfied[0] != 9 || intent.satisfied[1] != 5 {
		t.Fatalf("sampling intent = (%#v, %v), want ordered [9 5] with effective 1200", intent, err)
	}

	campaign.SmartTargetingTestSatisfiedTagIDs = pq.Int64Array{5, 9}
	if _, err := currentSmartTargetingTestSamplingIntent(t.Context(), repo, campaign, true); !errors.Is(err, ErrSmartTargetingTestPreviewRequired) {
		t.Fatalf("out-of-order intent error = %v, want preview required", err)
	}
}

func TestSmartTargetingTestSamplingConfigurationInvalidatesOnlyEffectiveChanges(t *testing.T) {
	method := models.CampaignAudienceTargetingSmart
	bundleID := uint(3)
	sampleSize := uint64(600)
	phase := string(models.CampaignPhaseTest)
	campaign := &models.Campaign{
		BundleID:         &bundleID,
		Phase:            models.CampaignPhaseTest,
		SampleSizePerTag: &sampleSize,
		Spec: models.CampaignSpec{
			AudienceTargetingMethod: &method,
			AudienceGrades:          []string{"A", "C"},
		},
	}

	unchanged := &dto.UpdateCampaignRequest{
		AudienceTargetingMethod: &method,
		BundleID:                &bundleID,
		Phase:                   &phase,
		SampleSizePerTag:        &sampleSize,
		AudienceGrades:          []string{"C", "A"},
	}
	changed, err := smartTargetingTestSamplingConfigurationChanged(campaign, unchanged)
	if err != nil || changed {
		t.Fatalf("unchanged effective sampling configuration = (%t, %v), want (false, nil)", changed, err)
	}

	changedSampleSize := uint64(601)
	changed, err = smartTargetingTestSamplingConfigurationChanged(campaign, &dto.UpdateCampaignRequest{SampleSizePerTag: &changedSampleSize})
	if err != nil || !changed {
		t.Fatalf("changed sample size = (%t, %v), want (true, nil)", changed, err)
	}

	execution := string(models.CampaignPhaseExecution)
	changed, err = smartTargetingTestSamplingConfigurationChanged(campaign, &dto.UpdateCampaignRequest{Phase: &execution})
	if err != nil || !changed {
		t.Fatalf("changed phase = (%t, %v), want (true, nil)", changed, err)
	}
}
