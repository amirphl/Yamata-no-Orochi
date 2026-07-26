package businessflow

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/lib/pq"
)

func TestNormalizeSmartTargetingScoreClasses(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
		err   bool
	}{
		{name: "omitted means all", want: []string{"A", "B", "C"}},
		{name: "canonical sort and case", input: []string{"c", "A"}, want: []string{"A", "C"}},
		{name: "duplicate rejected", input: []string{"A", "a"}, err: true},
		{name: "unknown rejected", input: []string{"D"}, err: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeSmartTargetingScoreClasses(tt.input)
			if tt.err {
				if !errors.Is(err, ErrSmartTargetingScoreClassesInvalid) {
					t.Fatalf("error = %v, want score class error", err)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("normalize = (%v, %v), want %v", got, err, tt.want)
			}
		})
	}
}

func TestSmartTargetingCapacityHashIsOrderIndependent(t *testing.T) {
	first := smartTargetingTagHash(18, []uint{2, 7, 9})
	second := smartTargetingTagHash(18, []uint{9, 2, 7})
	if first != second {
		t.Fatal("equal canonical tag selections must produce equal hashes")
	}
	if first == smartTargetingTagHash(19, []uint{2, 7, 9}) {
		t.Fatal("campaign ID must participate in the tag hash")
	}
	if smartTargetingInputHash(first, []string{"A", "B"}) == smartTargetingInputHash(first, []string{"A", "C"}) {
		t.Fatal("score classes must participate in the input hash")
	}
	if smartTargetingInputHash(first, []string{"A", "B"}) != smartTargetingInputHash(first, []string{"A", "B"}) {
		t.Fatal("equal targeting inputs must produce equal hashes")
	}
}

func TestSmartTargetingScoreClassBoundaries(t *testing.T) {
	p33, p66 := 10.0, 20.0
	for _, tt := range []struct {
		score *float64
		want  string
	}{
		{score: nil, want: "unscored"},
		{score: floatPtr(10), want: "C"},
		{score: floatPtr(11), want: "B"},
		{score: floatPtr(20), want: "B"},
		{score: floatPtr(21), want: "A"},
	} {
		if got := smartTargetingScoreClass(tt.score, &p33, &p66); got != tt.want {
			t.Fatalf("score class = %q, want %q", got, tt.want)
		}
	}
	// Equal percentile boundaries intentionally leave class B empty: values at
	// the shared boundary are C and values above it are A.
	if got := smartTargetingScoreClass(floatPtr(10), &p33, &p33); got != "C" {
		t.Fatalf("equal-boundary score class = %q, want C", got)
	}
	if got := smartTargetingScoreClass(floatPtr(11), &p33, &p33); got != "A" {
		t.Fatalf("equal-boundary score class = %q, want A", got)
	}
}

func TestCapacityDTODoesNotExposeStaleCounts(t *testing.T) {
	now := time.Now().UTC()
	row := &models.CampaignTargetingCapacityCalculation{
		ID: 7, CampaignID: 3, BundleID: 4, Status: models.CampaignTargetingCapacityCalculated,
		SelectedScoreClasses: pq.StringArray{"A", "B", "C"}, SelectedTagCount: 2,
		RawAudienceCount: 100, EligibleUniqueAudienceCount: 80, ApprovedCampaignDeduction: 90,
		UsableUniqueAudienceCount: 0, CreatedAt: now,
	}
	stale := capacityDTO(row, false, true)
	if stale.Status != "stale" || !stale.RecalculationRequired || stale.UsableUniqueCount != nil || stale.RawAudienceCount != nil {
		t.Fatalf("stale response leaked a valid result: %#v", stale)
	}
	current := capacityDTO(row, true, false)
	if current.UsableUniqueCount == nil || *current.UsableUniqueCount != 0 || current.ApprovedDeduction == nil || *current.ApprovedDeduction != 90 {
		t.Fatalf("current zero result must remain distinguishable from unavailable: %#v", current)
	}
}

func TestCapacityDTOStatesKeepPendingAndFailedCountsHidden(t *testing.T) {
	now := time.Now().UTC()
	message := "failed"
	for _, tt := range []struct {
		name   string
		status models.CampaignTargetingCapacityCalculationStatus
		want   string
	}{
		{name: "pending", status: models.CampaignTargetingCapacityCalculating, want: "calculating"},
		{name: "failed", status: models.CampaignTargetingCapacityFailed, want: "failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			row := &models.CampaignTargetingCapacityCalculation{
				ID: 1, Status: tt.status, RawAudienceCount: 100, EligibleUniqueAudienceCount: 80,
				UsableUniqueAudienceCount: 70, ErrorMessage: &message, CreatedAt: now,
			}
			response := capacityDTO(row, false, false)
			if response.Status != tt.want || response.RawAudienceCount != nil || response.UsableUniqueCount != nil {
				t.Fatalf("response = %#v", response)
			}
		})
	}
}

func TestCanCalculateSmartTargetingCapacityAcrossPreExecutionLifecycle(t *testing.T) {
	allowed := []models.CampaignStatus{
		models.CampaignStatusInitiated,
		models.CampaignStatusInProgress,
		models.CampaignStatusWaitingForApproval,
		models.CampaignStatusApproved,
	}
	for _, status := range allowed {
		if !canCalculateSmartTargetingCapacity(&models.Campaign{Status: status}) {
			t.Fatalf("status %q must allow exact-capacity refresh", status)
		}
	}
	for _, status := range []models.CampaignStatus{models.CampaignStatusRunning, models.CampaignStatusExecuted, models.CampaignStatusCancelled} {
		if canCalculateSmartTargetingCapacity(&models.Campaign{Status: status}) {
			t.Fatalf("status %q must not allow exact-capacity refresh", status)
		}
	}
}

func TestCalculationExpiryCoversScheduledExecution(t *testing.T) {
	now := time.Date(2026, time.August, 3, 10, 0, 0, 0, time.UTC)
	farSchedule := now.Add(30 * 24 * time.Hour)
	campaign := &models.Campaign{Spec: models.CampaignSpec{ScheduleAt: &farSchedule}}
	if got, want := calculationExpiry(now, campaign), farSchedule.Add(smartTargetingCapacityTTL); !got.Equal(want) {
		t.Fatalf("scheduled expiry = %s, want %s", got, want)
	}

	nearSchedule := now.Add(time.Hour)
	campaign.Spec.ScheduleAt = &nearSchedule
	if got, want := calculationExpiry(now, campaign), nearSchedule.Add(smartTargetingCapacityTTL); !got.Equal(want) {
		t.Fatalf("near-schedule expiry = %s, want %s", got, want)
	}
}

func floatPtr(value float64) *float64 { return &value }
