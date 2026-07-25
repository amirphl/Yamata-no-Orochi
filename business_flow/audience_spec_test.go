package businessflow

import (
	"testing"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/app/dto"
	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/amirphl/Yamata-no-Orochi/repository"
)

func audienceSpecTestRows() []repository.AudienceSpecSourceRow {
	return []repository.AudienceSpecSourceRow{
		{
			TagID:          20,
			Layer1Category: "level1",
			Layer2Category: "level2",
			Layer3Category: "level3",
			StatsFound:     true,
			DistinctUsers:  900,
			BlackUsers:     100,
			WhiteUsers:     300,
			PinkUsers:      301,
			WeakWhite:      101,
			GoodWhite:      102,
			BestWhite:      97,
			WeakBlack:      31,
			GoodBlack:      32,
			BestBlack:      37,
			WeakPink:       99,
			GoodPink:       100,
			BestPink:       102,
			ScoredUsers:    701,
		},
		{
			TagID:          3,
			Layer1Category: "level1",
			Layer2Category: "level2",
			Layer3Category: "level3",
			StatsFound:     true,
			DistinctUsers:  900,
			BlackUsers:     100,
			WhiteUsers:     300,
			PinkUsers:      301,
			WeakWhite:      101,
			GoodWhite:      102,
			BestWhite:      97,
			WeakBlack:      31,
			GoodBlack:      32,
			BestBlack:      37,
			WeakPink:       99,
			GoodPink:       100,
			BestPink:       102,
			ScoredUsers:    701,
		},
	}
}

func TestBuildAudienceSpecFromRowsSMSCapacity(t *testing.T) {
	spec, err := buildAudienceSpecFromRows(audienceSpecTestRows(), models.CampaignPlatformSMS)
	if err != nil {
		t.Fatalf("buildAudienceSpecFromRows returned error: %v", err)
	}
	leaf := spec["level1"]["level2"].Items["level3"]
	if leaf.AvailableAudience != 400 {
		t.Fatalf("SMS capacity = %d, want white(300) + pink(301)/3 = 400", leaf.AvailableAudience)
	}
	if len(leaf.Tags) != 2 || leaf.Tags[0] != "3" || leaf.Tags[1] != "20" {
		t.Fatalf("tags = %#v, want numerically sorted IDs [3 20]", leaf.Tags)
	}
	if leaf.DistinctUsers != 900 || leaf.BlackUsers != 100 || leaf.WhiteUsers != 300 || leaf.PinkUsers != 301 || leaf.ScoredUsers != 701 {
		t.Fatalf("user-count fields were not preserved: %#v", leaf)
	}
	if leaf.WeakWhite != 101 || leaf.GoodWhite != 102 || leaf.BestWhite != 97 ||
		leaf.WeakBlack != 31 || leaf.GoodBlack != 32 || leaf.BestBlack != 37 ||
		leaf.WeakPink != 99 || leaf.GoodPink != 100 || leaf.BestPink != 102 {
		t.Fatalf("color-grade fields were not preserved: %#v", leaf)
	}
}

func TestBuildAudienceSpecFromRowsNonSMSCapacity(t *testing.T) {
	for _, platform := range []string{
		models.CampaignPlatformBale,
		models.CampaignPlatformSPlus,
		models.CampaignPlatformRubika,
	} {
		t.Run(platform, func(t *testing.T) {
			spec, err := buildAudienceSpecFromRows(audienceSpecTestRows(), platform)
			if err != nil {
				t.Fatalf("buildAudienceSpecFromRows returned error: %v", err)
			}
			leaf := spec["level1"]["level2"].Items["level3"]
			if leaf.AvailableAudience != 701 {
				t.Fatalf("capacity = %d, want black(100) + white(300) + pink(301) = 701", leaf.AvailableAudience)
			}
		})
	}
}

func TestBuildAudienceSpecFromRowsRejectsConflictingStats(t *testing.T) {
	rows := audienceSpecTestRows()
	rows[1].WhiteUsers++
	if _, err := buildAudienceSpecFromRows(rows, models.CampaignPlatformSMS); err == nil {
		t.Fatal("expected conflicting hierarchy statistics to fail")
	}
}

func TestBuildAudienceSpecFromRowsRejectsNegativeStats(t *testing.T) {
	rows := audienceSpecTestRows()
	rows[0].PinkUsers = -1
	if _, err := buildAudienceSpecFromRows(rows, models.CampaignPlatformSMS); err == nil {
		t.Fatal("expected negative audience statistics to fail")
	}
}

func TestBuildAudienceSpecFromRowsRejectsConflictingReturnedUserCounts(t *testing.T) {
	rows := audienceSpecTestRows()
	rows[1].ScoredUsers++
	if _, err := buildAudienceSpecFromRows(rows, models.CampaignPlatformSMS); err == nil {
		t.Fatal("expected conflicting returned user counts to fail")
	}
}

func TestBuildAudienceSpecFromRowsRejectsMissingStats(t *testing.T) {
	rows := audienceSpecTestRows()
	rows[0].StatsFound = false
	if _, err := buildAudienceSpecFromRows(rows, models.CampaignPlatformSMS); err == nil {
		t.Fatal("expected an active tag without matching statistics to fail")
	}
}

func TestBuildAudienceSpecFromRowsRejectsEmptySource(t *testing.T) {
	if _, err := buildAudienceSpecFromRows(nil, models.CampaignPlatformSMS); err == nil {
		t.Fatal("expected an empty audience specification source to fail")
	}
}

func TestValidateAudienceSpecRejectsInconsistentCachedCapacity(t *testing.T) {
	spec, err := buildAudienceSpecFromRows(audienceSpecTestRows(), models.CampaignPlatformSMS)
	if err != nil {
		t.Fatalf("buildAudienceSpecFromRows returned error: %v", err)
	}
	level2 := spec["level1"]["level2"]
	item := level2.Items["level3"]
	item.AvailableAudience++
	level2.Items["level3"] = item
	spec["level1"]["level2"] = level2

	if err := validateAudienceSpec(spec, models.CampaignPlatformSMS); err == nil {
		t.Fatal("expected inconsistent cached capacity to fail validation")
	}
}

func TestCalculateAudienceSpecItemCapacityRespectsSelectedGrades(t *testing.T) {
	item := buildAudienceSpecTestItem(t, models.CampaignPlatformSMS)
	capacity, grades, err := calculateAudienceSpecItemCapacity(models.CampaignPlatformSMS, []string{audienceGradeA, audienceGradeC}, item)
	if err != nil {
		t.Fatalf("calculateAudienceSpecItemCapacity returned error: %v", err)
	}

	if grades[audienceGradeA] != 131 || grades[audienceGradeB] != 135 || grades[audienceGradeC] != 134 {
		t.Fatalf("grade capacities = %#v, want A=131 B=135 C=134", grades)
	}
	if capacity != 265 {
		t.Fatalf("selected capacity = %d, want A(131) + C(134) = 265", capacity)
	}
}

func TestCalculateAudienceSpecItemCapacityDefaultsToAllGrades(t *testing.T) {
	item := buildAudienceSpecTestItem(t, models.CampaignPlatformBale)
	capacity, grades, err := calculateAudienceSpecItemCapacity(models.CampaignPlatformBale, nil, item)
	if err != nil {
		t.Fatalf("calculateAudienceSpecItemCapacity returned error: %v", err)
	}

	if grades[audienceGradeA] != 236 || grades[audienceGradeB] != 234 || grades[audienceGradeC] != 231 {
		t.Fatalf("grade capacities = %#v, want A=236 B=234 C=231", grades)
	}
	if capacity != 701 {
		t.Fatalf("selected capacity = %d, want 701", capacity)
	}
}

func TestCalculateAudienceSpecItemCapacityRejectsInvalidStats(t *testing.T) {
	item := buildAudienceSpecTestItem(t, models.CampaignPlatformSMS)
	item.BestPink = -1
	if _, _, err := calculateAudienceSpecItemCapacity(models.CampaignPlatformSMS, nil, item); err == nil {
		t.Fatal("expected negative grade statistics to fail")
	}
}

func buildAudienceSpecTestItem(t *testing.T, platform string) dto.AudienceSpecItem {
	t.Helper()
	spec, err := buildAudienceSpecFromRows(audienceSpecTestRows(), platform)
	if err != nil {
		t.Fatalf("buildAudienceSpecFromRows returned error: %v", err)
	}
	return spec["level1"]["level2"].Items["level3"]
}

func TestAudienceSpecCacheTTLIsFiveMinutes(t *testing.T) {
	if audienceSpecCacheTTL != 5*time.Minute {
		t.Fatalf("audienceSpecCacheTTL = %s, want 5m", audienceSpecCacheTTL)
	}
}
