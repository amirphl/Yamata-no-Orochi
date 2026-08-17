package scheduler

import (
	"context"
	"io"
	"log"
	"strings"
	"testing"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
	"github.com/lib/pq"
)

type missingSchedulerStatsRepository struct{}

func (missingSchedulerStatsRepository) FetchPercentiles(context.Context, *string, []string, []string) (*repository.LayerPercentiles, error) {
	return nil, nil
}

type processedCampaignStatsRepositoryStub struct {
	repository.ProcessedCampaignRepository
	updated *models.ProcessedCampaign
}

func (s *processedCampaignStatsRepositoryStub) UpdateMeta(_ context.Context, campaign *models.ProcessedCampaign) error {
	copy := *campaign
	s.updated = &copy
	return nil
}

func TestPreparedCampaignStatisticsPersistsExplicitZeroAudienceSnapshot(t *testing.T) {
	repo := &processedCampaignStatsRepositoryStub{}
	processed := &models.ProcessedCampaign{ID: 19}
	updateCalled := false
	stats, err := preparedCampaignStatistics(t.Context(), repo, processed, 0, func(context.Context, uint) (map[string]any, error) {
		updateCalled = true
		return nil, nil
	})
	if err != nil {
		t.Fatalf("persist zero-audience statistics: %v", err)
	}
	if updateCalled {
		t.Fatal("provider aggregation must not run for an empty prepared audience")
	}
	if repo.updated == nil || len(repo.updated.Statistics) == 0 {
		t.Fatal("zero-audience statistics were not persisted to the processed campaign")
	}
	if sent, ok := stats["aggregatedTotalSent"].(int64); !ok || sent != 0 {
		t.Fatalf("aggregatedTotalSent = %#v, want int64(0)", stats["aggregatedTotalSent"])
	}
	if !shouldPushPreparedCampaignStatistics(stats, 0) {
		t.Fatal("explicit zero-audience statistics must be pushed to the campaign service")
	}
}

func TestUsesExcelAudienceTargetingBackwardCompatibilityAndPriority(t *testing.T) {
	excelUUID := "4a54766e-4330-4cff-8658-bcd3c742b469"

	if !usesExcelAudienceTargeting(dto.BotGetCampaignResponse{TargetAudienceExcelFileUUID: &excelUUID}) {
		t.Fatal("legacy bot response with an Excel UUID must use Excel targeting")
	}
	if !usesExcelAudienceTargeting(dto.BotGetCampaignResponse{
		TargetingMethod:             models.CampaignAudienceTargetingExcel,
		TargetAudienceExcelFileUUID: &excelUUID,
	}) {
		t.Fatal("explicit Excel targeting must use Excel")
	}
	if usesExcelAudienceTargeting(dto.BotGetCampaignResponse{
		TargetingMethod:             models.CampaignAudienceTargetingSmart,
		TargetAudienceExcelFileUUID: &excelUUID,
	}) {
		t.Fatal("Smart targeting must take priority over a stale Excel UUID")
	}
	if usesExcelAudienceTargeting(dto.BotGetCampaignResponse{
		TargetingMethod:             models.CampaignAudienceTargetingStandard,
		TargetAudienceExcelFileUUID: &excelUUID,
	}) {
		t.Fatal("Standard targeting must take priority over a stale Excel UUID")
	}
}

func TestCampaignExecutionTagsKeepsSmartTagsSeparate(t *testing.T) {
	got := campaignExecutionTags(dto.BotGetCampaignResponse{
		TargetingMethod: models.CampaignAudienceTargetingSmart,
		Tags:            []string{"1"},
		SelectedTags:    []string{"7", "9"},
	})
	if len(got) != 2 || got[0] != "7" || got[1] != "9" {
		t.Fatalf("smart targeting must use selected_tags, got %#v", got)
	}

	got = campaignExecutionTags(dto.BotGetCampaignResponse{
		TargetingMethod: models.CampaignAudienceTargetingStandard,
		Tags:            []string{"1"},
		SelectedTags:    []string{"7"},
	})
	if len(got) != 1 || got[0] != "1" {
		t.Fatalf("standard targeting must keep using tags, got %#v", got)
	}
}

func TestCampaignIgnoresAudienceGradesOnlyForTag17358(t *testing.T) {
	if !campaignIgnoresAudienceGrades(dto.BotGetCampaignResponse{Tags: []string{"17358"}}) {
		t.Fatal("tag 17358 must bypass audience grades")
	}
	if !campaignIgnoresAudienceGrades(dto.BotGetCampaignResponse{Tags: []string{" 17358 ", "17358"}}) {
		t.Fatal("duplicate tag 17358 values must bypass audience grades")
	}
	if campaignIgnoresAudienceGrades(dto.BotGetCampaignResponse{Tags: []string{"17358", "9"}}) {
		t.Fatal("grade bypass must not leak to campaigns targeting other tags")
	}
	if campaignIgnoresAudienceGrades(dto.BotGetCampaignResponse{Tags: []string{"9"}}) {
		t.Fatal("tags other than 17358 must retain audience grades")
	}
}

func TestParseCampaignTagIDsRejectsInvalidAndDeduplicates(t *testing.T) {
	_, ids, err := parseCampaignTagIDs(dto.BotGetCampaignResponse{ID: 10, Tags: []string{"7", "7", "9"}})
	if err != nil {
		t.Fatalf("parse valid tags: %v", err)
	}
	if len(ids) != 2 || ids[0] != 7 || ids[1] != 9 {
		t.Fatalf("parsed tag IDs = %v, want [7 9]", ids)
	}

	for _, campaign := range []dto.BotGetCampaignResponse{
		{ID: 11},
		{ID: 12, Tags: []string{"not-an-id"}},
		{ID: 13, Tags: []string{"0"}},
	} {
		if _, _, err := parseCampaignTagIDs(campaign); err == nil {
			t.Fatalf("campaign %d: expected invalid tags to fail", campaign.ID)
		}
	}
}

func TestRequireAllTagsActiveFailsClosed(t *testing.T) {
	if _, err := requireAllTagsActive(746, []uint{17358}, nil); err == nil || !strings.Contains(err.Error(), "17358") {
		t.Fatalf("inactive tag must produce a descriptive error, got %v", err)
	}

	if _, err := requireAllTagsActive(747, []uint{7, 9}, []*models.Tag{{ID: 7}}); err == nil || !strings.Contains(err.Error(), "9") {
		t.Fatalf("partially resolved tags must fail closed, got %v", err)
	}

	resolved, err := requireAllTagsActive(748, []uint{7, 9}, []*models.Tag{{ID: 9}, {ID: 7}})
	if err != nil {
		t.Fatalf("active tags unexpectedly failed: %v", err)
	}
	if len(resolved) != 2 || resolved[0] != 7 || resolved[1] != 9 {
		t.Fatalf("resolved tag IDs = %v, want [7 9]", resolved)
	}
}

func TestRequireExactAudienceCountRejectsPartialSelection(t *testing.T) {
	if err := requireExactAudienceCount(746, 50_000, 49_999); err == nil || !strings.Contains(err.Error(), "requires exactly 50000 audiences") {
		t.Fatalf("partial audience selection must fail with requested and selected counts, got %v", err)
	}
	if err := requireExactAudienceCount(746, 50_000, 50_000); err != nil {
		t.Fatalf("exact audience selection unexpectedly failed: %v", err)
	}
	if err := requireAudienceMatch(745, pq.Int32Array{7}, 1); err != nil {
		t.Fatalf("legacy non-bundle partial selection must remain valid: %v", err)
	}
	if err := requireAudienceMatch(745, pq.Int32Array{7}, 0); err == nil {
		t.Fatal("legacy non-bundle empty selection must fail")
	}
}

func TestShouldPushCurrentProcessedCampaignStatistics(t *testing.T) {
	positive := map[string]any{"aggregatedTotalSent": int64(1)}
	zero := map[string]any{"aggregatedTotalSent": int64(0)}

	if shouldPushCurrentProcessedCampaignStatistics(nil, positive) {
		t.Fatal("nil processed campaign must not publish statistics")
	}
	if shouldPushCurrentProcessedCampaignStatistics(&models.ProcessedCampaign{IsCurrent: false}, positive) {
		t.Fatal("historical processed campaign must not publish statistics")
	}
	if shouldPushCurrentProcessedCampaignStatistics(&models.ProcessedCampaign{IsCurrent: true}, nil) {
		t.Fatal("nil statistics must not be published")
	}
	if shouldPushCurrentProcessedCampaignStatistics(&models.ProcessedCampaign{IsCurrent: true}, zero) {
		t.Fatal("zero-sent statistics must not be published")
	}
	if !shouldPushCurrentProcessedCampaignStatistics(&models.ProcessedCampaign{IsCurrent: true}, positive) {
		t.Fatal("current processed campaign with sent messages must publish statistics")
	}
}

func TestExcelTargetingPreservesReusableEmptyAudienceBehavior(t *testing.T) {
	result, err := fetchAudiencePhonesByUIDs(
		context.Background(),
		log.New(io.Discard, "", 0),
		nil,
		nil,
		dto.BotGetCampaignResponse{ID: 747},
		"",
		nil,
		"",
	)
	if err != nil {
		t.Fatalf("empty reusable Excel audience unexpectedly failed: %v", err)
	}
	if result == nil || len(result.IDs) != 0 || result.BundleAudienceSelectionID != nil {
		t.Fatalf("Excel targeting must remain outside selection caches: %#v", result)
	}
}

func TestExcelTargetingCanReuseAudienceAcrossBundleCampaigns(t *testing.T) {
	bundleID := uint(44)
	phone := "09120000001"
	repo := &stubSMSAudienceProfileRepo{byUIDsFn: func(_ context.Context, uids []string) ([]*models.AudienceProfile, error) {
		if len(uids) != 1 || uids[0] != "uid-1" {
			t.Fatalf("unexpected Excel UIDs: %v", uids)
		}
		return []*models.AudienceProfile{{ID: 91, UID: "uid-1", PhoneNumber: &phone}}, nil
	}}
	for attempt := 0; attempt < 2; attempt++ {
		campaign := dto.BotGetCampaignResponse{ID: uint(748 + attempt), BundleID: &bundleID, TargetingMethod: models.CampaignAudienceTargetingExcel}
		result, err := fetchAudiencePhonesByUIDs(context.Background(), log.New(io.Discard, "", 0), repo, nil, campaign, "", []string{"uid-1"}, "")
		if err != nil {
			t.Fatalf("Excel selection attempt %d failed: %v", attempt+1, err)
		}
		if len(result.IDs) != 1 || result.IDs[0] != 91 || result.BundleAudienceSelectionID != nil {
			t.Fatalf("Excel selection attempt %d unexpectedly entered selection cache: %#v", attempt+1, result)
		}
	}
}

func TestStandardScoreResolutionOnlyRequiresStatisticsForRestrictedGrades(t *testing.T) {
	t.Parallel()
	logger := log.New(io.Discard, "", 0)
	bypassCampaigns := []dto.BotGetCampaignResponse{
		{ID: 812, Tags: []string{"7"}, AudienceGrades: []string{"A", "B", "C"}},
		{ID: 813, Tags: []string{"17358"}, AudienceGrades: []string{"A"}},
	}
	restricted := dto.BotGetCampaignResponse{ID: 814, Tags: []string{"7"}, AudienceGrades: []string{"A"}}

	resolvers := []struct {
		name    string
		resolve func(context.Context, dto.BotGetCampaignResponse) (*models.NormalizedScoreConstraint, error)
	}{
		{name: "sms", resolve: (&SMSCampaignScheduler{statsRepo: missingSchedulerStatsRepository{}, logger: logger}).resolveScoreConstraint},
		{name: "bale", resolve: (&BaleCampaignScheduler{statsRepo: missingSchedulerStatsRepository{}, logger: logger}).resolveScoreConstraint},
		{name: "rubika", resolve: (&RubikaCampaignScheduler{statsRepo: missingSchedulerStatsRepository{}, logger: logger}).resolveScoreConstraint},
		{name: "splus", resolve: (&SplusCampaignScheduler{statsRepo: missingSchedulerStatsRepository{}, logger: logger}).resolveScoreConstraint},
	}

	for _, tt := range resolvers {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, campaign := range bypassCampaigns {
				if constraint, err := tc.resolve(context.Background(), campaign); err != nil || constraint != nil {
					t.Fatalf("campaign %d: no score filter should be required, constraint=%v err=%v", campaign.ID, constraint, err)
				}
			}
			if _, err := tc.resolve(context.Background(), restricted); err == nil || !strings.Contains(err.Error(), "statistics are missing") {
				t.Fatalf("restricted grades without statistics must fail, got %v", err)
			}
		})
	}
}

func TestBundleSelectionIDFromAudienceResult(t *testing.T) {
	ptr := func(value uint) *uint { return &value }

	tests := []struct {
		name    string
		result  *AudiencePhonesResult
		wantID  *uint
		wantErr bool
	}{
		{
			name:   "valid selection",
			result: &AudiencePhonesResult{BundleAudienceSelectionID: ptr(22)},
			wantID: ptr(22),
		},
		{
			name:    "nil result",
			result:  nil,
			wantErr: true,
		},
		{
			name:    "missing selection",
			result:  &AudiencePhonesResult{},
			wantErr: true,
		},
		{
			name:    "zero selection",
			result:  &AudiencePhonesResult{BundleAudienceSelectionID: ptr(0)},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selectionID, err := bundleSelectionIDFromAudienceResult(tt.result)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected bundle selection validation to fail")
				}
				return
			}
			if err != nil {
				t.Fatalf("bundle selection validation failed: %v", err)
			}
			if !equalOptionalUint(selectionID, tt.wantID) {
				t.Fatalf("bundle audience selection id = %v, want %v", selectionID, tt.wantID)
			}
		})
	}
}

func equalOptionalUint(got, want *uint) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return *got == *want
}
