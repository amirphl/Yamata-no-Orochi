package repository

import (
	"strings"
	"testing"
)

func TestSmartTargetingPopulationUsesOptionalParameterizedColorFilter(t *testing.T) {
	query := strings.ToLower(smartTargetingPopulationCTE)
	for _, forbidden := range []string{"platform", "'white'", "'pink'", "candidate_stack", "insert into"} {
		if strings.Contains(query, forbidden) {
			t.Fatalf("capacity population unexpectedly contains %q:\n%s", forbidden, smartTargetingPopulationCTE)
		}
	}
	for _, required := range []string{"bundle_audience_selection_members", "used.bundle_id", "used.audience_id", "percentile_disc", "ap.color = any(?::text[])"} {
		if !strings.Contains(query, required) {
			t.Fatalf("capacity population is missing %q:\n%s", required, smartTargetingPopulationCTE)
		}
	}
}

func TestSmartTargetingPopulationArgsDisableEmptyColorFilter(t *testing.T) {
	args := smartTargetingPopulationArgs(SmartTargetingAudienceQuery{TagIDs: []int64{7}, BundleID: 3})
	if disabled, ok := args[1].(bool); !ok || !disabled {
		t.Fatalf("empty allowed colors produced disabled=%#v, want true", args[1])
	}

	args = smartTargetingPopulationArgs(SmartTargetingAudienceQuery{
		TagIDs: []int64{7}, BundleID: 3, AllowedColors: []string{"white", "pink"},
	})
	if disabled, ok := args[1].(bool); !ok || disabled {
		t.Fatalf("SMS allowed colors produced disabled=%#v, want false", args[1])
	}
}

func TestSmartTargetingAllClassesRequiresCanonicalDistinctSet(t *testing.T) {
	if !smartTargetingAllClasses([]string{"A", "B", "C"}) {
		t.Fatal("canonical A/B/C selection must use the all-classes query")
	}
	for _, classes := range [][]string{{"A", "A", "A"}, {"C", "B", "A"}, {"A", "B"}} {
		if smartTargetingAllClasses(classes) {
			t.Fatalf("invalid or non-canonical classes %v used the all-classes query", classes)
		}
	}
}
