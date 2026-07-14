package scheduler

import (
	"testing"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
)

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

func TestSelectionIDsForCampaignEnforcesSelectionScope(t *testing.T) {
	ptr := func(value uint) *uint { return &value }
	bundleID := uint(42)

	tests := []struct {
		name           string
		campaign       dto.BotGetCampaignResponse
		result         *AudiencePhonesResult
		wantAudienceID *uint
		wantBundleID   *uint
		wantErr        bool
	}{
		{
			name:           "non-bundle selection",
			result:         &AudiencePhonesResult{AudienceSelectionID: ptr(11)},
			wantAudienceID: ptr(11),
		},
		{
			name:         "bundle selection",
			campaign:     dto.BotGetCampaignResponse{BundleID: &bundleID},
			result:       &AudiencePhonesResult{BundleAudienceSelectionID: ptr(22)},
			wantBundleID: ptr(22),
		},
		{
			name:    "nil result",
			result:  nil,
			wantErr: true,
		},
		{
			name:    "non-bundle missing selection",
			result:  &AudiencePhonesResult{},
			wantErr: true,
		},
		{
			name:    "non-bundle zero selection",
			result:  &AudiencePhonesResult{AudienceSelectionID: ptr(0)},
			wantErr: true,
		},
		{
			name:    "non-bundle carrying bundle selection",
			result:  &AudiencePhonesResult{BundleAudienceSelectionID: ptr(22)},
			wantErr: true,
		},
		{
			name:    "non-bundle carrying both selections",
			result:  &AudiencePhonesResult{AudienceSelectionID: ptr(11), BundleAudienceSelectionID: ptr(22)},
			wantErr: true,
		},
		{
			name:     "bundle missing selection",
			campaign: dto.BotGetCampaignResponse{BundleID: &bundleID},
			result:   &AudiencePhonesResult{},
			wantErr:  true,
		},
		{
			name:     "bundle zero selection",
			campaign: dto.BotGetCampaignResponse{BundleID: &bundleID},
			result:   &AudiencePhonesResult{BundleAudienceSelectionID: ptr(0)},
			wantErr:  true,
		},
		{
			name:     "bundle carrying non-bundle selection",
			campaign: dto.BotGetCampaignResponse{BundleID: &bundleID},
			result:   &AudiencePhonesResult{AudienceSelectionID: ptr(11)},
			wantErr:  true,
		},
		{
			name:     "bundle carrying both selections",
			campaign: dto.BotGetCampaignResponse{BundleID: &bundleID},
			result:   &AudiencePhonesResult{AudienceSelectionID: ptr(11), BundleAudienceSelectionID: ptr(22)},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			audienceID, bundleSelectionID, err := selectionIDsForCampaign(tt.campaign, tt.result)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected selection scope validation to fail")
				}
				return
			}
			if err != nil {
				t.Fatalf("selection scope validation failed: %v", err)
			}
			if !equalOptionalUint(audienceID, tt.wantAudienceID) {
				t.Fatalf("audience selection id = %v, want %v", audienceID, tt.wantAudienceID)
			}
			if !equalOptionalUint(bundleSelectionID, tt.wantBundleID) {
				t.Fatalf("bundle audience selection id = %v, want %v", bundleSelectionID, tt.wantBundleID)
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
