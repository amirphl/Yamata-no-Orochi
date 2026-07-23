package repository

import (
	"strings"
	"testing"
)

func TestSmartTargetingPopulationIsPlatformIndependentAndNotMaterialized(t *testing.T) {
	query := strings.ToLower(smartTargetingPopulationCTE)
	for _, forbidden := range []string{"platform", "ap.color", "candidate_stack", "insert into"} {
		if strings.Contains(query, forbidden) {
			t.Fatalf("capacity population unexpectedly contains %q:\n%s", forbidden, smartTargetingPopulationCTE)
		}
	}
	for _, required := range []string{"bundle_audience_selection_members", "used.bundle_id", "used.audience_id", "percentile_disc"} {
		if !strings.Contains(query, required) {
			t.Fatalf("capacity population is missing %q:\n%s", required, smartTargetingPopulationCTE)
		}
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
