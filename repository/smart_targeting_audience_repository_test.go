package repository

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/amirphl/Yamata-no-Orochi/models"
)

func TestSmartTargetingPopulationUsesOptionalParameterizedColorFilter(t *testing.T) {
	query := strings.ToLower(smartTargetingPopulationCTE)
	for _, forbidden := range []string{"platform", "'white'", "'pink'", "candidate_stack", "insert into"} {
		if strings.Contains(query, forbidden) {
			t.Fatalf("capacity population unexpectedly contains %q:\n%s", forbidden, smartTargetingPopulationCTE)
		}
	}
	for _, required := range []string{"bundle_audience_selection_members", "used.bundle_id", "used.audience_id", "bundle_audience_exclusions", "bundle_exclusion.bundle_id", "bundle_exclusion.audience_id", "percentile_disc", "ap.color = any(?::text[])"} {
		if !strings.Contains(query, required) {
			t.Fatalf("capacity population is missing %q:\n%s", required, smartTargetingPopulationCTE)
		}
	}
	for _, forbidden := range []string{"ap.uid", "ap.phone_number,", "ap.tags,"} {
		if strings.Contains(query, forbidden) {
			t.Fatalf("capacity population materializes unused column %q:\n%s", forbidden, smartTargetingPopulationCTE)
		}
	}
	selectionQuery := strings.ToLower(smartTargetingSelectionPopulationCTE)
	for _, required := range []string{"ap.uid", "ap.phone_number", "ap.tags", "ap.normalized_score"} {
		if !strings.Contains(selectionQuery, required) {
			t.Fatalf("execution selection population is missing %q:\n%s", required, smartTargetingSelectionPopulationCTE)
		}
	}
}

func TestSmartTargetingPopulationArgsControlOptionalEligibilityFilters(t *testing.T) {
	args := smartTargetingPopulationArgs(SmartTargetingAudienceQuery{TagIDs: []int64{7}, BundleID: 3})
	if disabled, ok := args[1].(bool); !ok || !disabled {
		t.Fatalf("empty allowed colors produced disabled=%#v, want true", args[1])
	}
	if disabled, ok := args[4].(bool); !ok || !disabled {
		t.Fatalf("default Bundle exclusions produced disabled=%#v, want true", args[4])
	}

	args = smartTargetingPopulationArgs(SmartTargetingAudienceQuery{
		TagIDs: []int64{7}, BundleID: 3, AllowedColors: []string{"white", "pink"}, ApplyBundleAudienceExclusions: true,
	})
	if disabled, ok := args[1].(bool); !ok || disabled {
		t.Fatalf("SMS allowed colors produced disabled=%#v, want false", args[1])
	}
	if disabled, ok := args[4].(bool); !ok || disabled {
		t.Fatalf("enabled Bundle exclusions produced disabled=%#v, want false", args[4])
	}
	if len(args) != 6 || args[3] != uint(3) || args[5] != uint(3) {
		t.Fatalf("population arguments = %#v, want colors and Bundle eligibility inputs", args)
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

func TestSmartTargetingClassifiedPopulationCalculatesBothBoundsWithOneAggregate(t *testing.T) {
	query := strings.ToLower(smartTargetingClassifiedPopulationCTE)
	if got := strings.Count(query, "percentile_disc"); got != 1 {
		t.Fatalf("classified population percentile aggregate count = %d, want 1:\n%s", got, query)
	}
	for _, required := range []string{"array[0.33, 0.66]", "percentile_values[1] as p33", "percentile_values[2] as p66"} {
		if !strings.Contains(query, required) {
			t.Fatalf("classified population is missing %q:\n%s", required, query)
		}
	}
}

func TestSmartTargetingScoreBoundsQueryCalculatesEligibleUnionOnce(t *testing.T) {
	query := SmartTargetingAudienceQuery{
		BundleID: 3, ApplyBundleAudienceExclusions: true, TagIDs: []int64{9, 2}, ScoreClasses: []string{"A", "C"}, AllowedColors: []string{"white", "pink"},
	}
	sql, args := smartTargetingScoreBoundsQuery(query)
	lowerSQL := strings.ToLower(sql)
	for _, required := range []string{
		"percentile_disc(array[0.33, 0.66]", "ap.tags && ?::integer[]", "ap.color = any(?::text[])",
		"bundle_audience_selection_members", "used.bundle_id = ?", "ap.normalized_score is not null",
		"bundle_audience_exclusions", "bundle_exclusion.bundle_id = ?", "bundle_exclusion.audience_id = ap.id",
	} {
		if !strings.Contains(lowerSQL, required) {
			t.Fatalf("score-bound query is missing %q:\n%s", required, sql)
		}
	}
	if strings.Count(lowerSQL, "percentile_disc") != 1 {
		t.Fatalf("score-bound query recalculates percentile aggregates:\n%s", sql)
	}
	if len(args) != 4 || args[3] != uint(3) {
		t.Fatalf("score-bound arguments = %#v, want tag IDs, colors, and Bundle twice", args)
	}

	withoutColors, args := smartTargetingScoreBoundsQuery(SmartTargetingAudienceQuery{
		BundleID: 3, TagIDs: []int64{9}, ScoreClasses: []string{"A"},
	})
	if strings.Contains(strings.ToLower(withoutColors), "ap.color") || len(args) != 2 {
		t.Fatalf("unrestricted-color score-bound query = %q with %d args", withoutColors, len(args))
	}
	if strings.Contains(strings.ToLower(withoutColors), "bundle_audience_exclusions") {
		t.Fatalf("default score-bound query unexpectedly applies Bundle exclusions:\n%s", withoutColors)
	}
}

func TestSmartTargetingPerTagSelectionUsesIndexedRandomWindow(t *testing.T) {
	p33, p66 := 24.0, 29.6
	pivot := int64(-7_000_000_000_000_000_000)
	query := SmartTargetingAudienceQuery{
		BundleID: 3, ApplyBundleAudienceExclusions: true, TagIDs: []int64{9, 2}, ScoreClasses: []string{"A", "C"},
	}
	sql, args, err := smartTargetingPerTagSelectionQuery(query, &SmartTargetingScoreBounds{P33: &p33, P66: &p66}, 9, 4_800, true, pivot, false, nil)
	if err != nil {
		t.Fatalf("build per-tag selection query: %v", err)
	}
	lowerSQL := strings.ToLower(sql)
	for _, required := range []string{
		"select ap.id", "hashint8extended(ap.id, 0) as sample_key", "from audience_profiles as ap", "hashint8extended(ap.id, 0) >= ?::bigint",
		"ap.tags @> array[9]::integer[]", "bundle_audience_selection_members",
		"bundle_audience_exclusions", "bundle_exclusion.bundle_id = ?", "bundle_exclusion.audience_id = ap.id",
		"offset 0", "ap.normalized_score <= ?::double precision or ap.normalized_score > ?::double precision",
		"order by hashint8extended(ap.id, 0) asc, ap.id asc", "limit ?",
	} {
		if !strings.Contains(lowerSQL, required) {
			t.Fatalf("per-tag selection query is missing %q:\n%s", required, sql)
		}
	}
	for _, forbidden := range []string{"order by random", "order by normalized_score", "= any(tags)", "array[?]", "candidate_population", "classified", "ap.uid", "unnest("} {
		if strings.Contains(lowerSQL, forbidden) {
			t.Fatalf("per-tag ID query unexpectedly contains %q:\n%s", forbidden, sql)
		}
	}
	if len(args) != 6 || args[1] != uint(3) || args[0] != pivot {
		t.Fatalf("per-tag arguments = %#v, want pivot, Bundle twice, two bounds, and limit", args)
	}

	fullSQL, _, err := smartTargetingPerTagSelectionQuery(query, &SmartTargetingScoreBounds{P33: &p33, P66: &p66}, 9, 4_800, false, pivot, false, nil)
	if err != nil {
		t.Fatalf("build full per-tag selection query: %v", err)
	}
	for _, column := range []string{"ap.uid", "ap.phone_number", "ap.tags", "ap.normalized_score"} {
		if !strings.Contains(strings.ToLower(fullSQL), column) {
			t.Fatalf("final scheduler query is missing %q:\n%s", column, fullSQL)
		}
	}
	if strings.Contains(strings.ToLower(fullSQL), "order by random") {
		t.Fatalf("final per-tag scheduler query performs a full random sort:\n%s", fullSQL)
	}
	wrapSQL, _, err := smartTargetingPerTagSelectionQuery(query, &SmartTargetingScoreBounds{P33: &p33, P66: &p66}, 9, 4_800, true, pivot, true, nil)
	if err != nil || !strings.Contains(strings.ToLower(wrapSQL), "hashint8extended(ap.id, 0) < ?::bigint") {
		t.Fatalf("wrapped sample query = %q, %v", wrapSQL, err)
	}
	cursor := &smartTargetingSampleCursor{key: pivot + 100, id: 123}
	seekSQL, seekArgs, err := smartTargetingPerTagSelectionQuery(query, &SmartTargetingScoreBounds{P33: &p33, P66: &p66}, 9, 4_800, true, pivot, true, cursor)
	if err != nil || !strings.Contains(strings.ToLower(seekSQL), "(hashint8extended(ap.id, 0), ap.id) > (?::bigint, ?::bigint)") || !strings.Contains(strings.ToLower(seekSQL), "hashint8extended(ap.id, 0) < ?::bigint") {
		t.Fatalf("wrapped seek query = %q, %v", seekSQL, err)
	}
	if len(seekArgs) != 8 || seekArgs[0] != cursor.key || seekArgs[1] != cursor.id || seekArgs[2] != pivot {
		t.Fatalf("wrapped seek arguments = %#v", seekArgs)
	}
	if _, _, err := smartTargetingPerTagSelectionQuery(query, &SmartTargetingScoreBounds{P33: &p33, P66: &p66}, 77, 600, true, pivot, false, nil); err == nil {
		t.Fatal("per-tag selection accepted a tag outside the selected set")
	}
	query.TagIDs = append(query.TagIDs, int64(math.MaxInt32)+1)
	if _, _, err := smartTargetingPerTagSelectionQuery(query, &SmartTargetingScoreBounds{P33: &p33, P66: &p66}, int64(math.MaxInt32)+1, 600, true, pivot, false, nil); err == nil {
		t.Fatal("per-tag selection accepted a tag that cannot fit PostgreSQL integer[]")
	}
	query.ApplyBundleAudienceExclusions = false
	withoutBundleExclusions, args, err := smartTargetingPerTagSelectionQuery(query, &SmartTargetingScoreBounds{P33: &p33, P66: &p66}, 9, 600, true, pivot, false, nil)
	if err != nil {
		t.Fatalf("build default per-tag selection query: %v", err)
	}
	if strings.Contains(strings.ToLower(withoutBundleExclusions), "bundle_audience_exclusions") || len(args) != 5 {
		t.Fatalf("default per-tag query unexpectedly applies Bundle exclusions:\n%s\nargs=%#v", withoutBundleExclusions, args)
	}
}

func TestSmartTargetingSamplePivotIsStablePerPreviewAndTag(t *testing.T) {
	const seed = "ca0eebb1b9af1ad2a24050dfd76b04fd98b1db173a486748004dd728ea9ecb41"
	first, err := smartTargetingSamplePivot(seed, 9)
	if err != nil {
		t.Fatal(err)
	}
	second, err := smartTargetingSamplePivot(seed, 9)
	if err != nil {
		t.Fatal(err)
	}
	otherTag, err := smartTargetingSamplePivot(seed, 10)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("same preview seed and tag produced pivots %d and %d", first, second)
	}
	if first == otherTag {
		t.Fatalf("different tags unexpectedly produced the same pivot %d", first)
	}
	if _, err := smartTargetingSamplePivot("", 9); err == nil {
		t.Fatal("empty persisted preview seed must fail closed")
	}
}

func TestSmartTargetingSamplePoolIsBoundedAndShuffleIsDeterministicForPivot(t *testing.T) {
	if got := smartTargetingSamplePoolLimit(600); got != 1_200 {
		t.Fatalf("sample pool limit = %d, want 1200", got)
	}
	if got := smartTargetingSamplePoolLimit(math.MaxInt64); got != math.MaxInt64 {
		t.Fatalf("overflow-safe sample pool limit = %d, want MaxInt64", got)
	}

	pivot := int64(0x0123456789abcdef)
	first := []*models.AudienceProfile{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5}}
	second := []*models.AudienceProfile{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5}}
	shuffleSmartTargetingSample(first, pivot)
	shuffleSmartTargetingSample(second, pivot)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same sampling pivot produced different shuffles: %#v != %#v", first, second)
	}
	seen := make(map[int64]bool, len(first))
	for _, row := range first {
		seen[row.ID] = true
	}
	for id := int64(1); id <= 5; id++ {
		if !seen[id] {
			t.Fatalf("shuffle lost audience %d: %#v", id, first)
		}
	}
}

func TestSmartTargetingPerTagScorePredicatesPreserveClassBoundaries(t *testing.T) {
	p33, p66 := 24.0, 29.6
	bounds := &SmartTargetingScoreBounds{P33: &p33, P66: &p66}
	tests := []struct {
		classes  []string
		wantSQL  string
		wantArgs []any
	}{
		{[]string{"A"}, "normalized_score >", []any{p66}},
		{[]string{"B"}, "normalized_score >", []any{p33, p66}},
		{[]string{"C"}, "normalized_score <=", []any{p33}},
		{[]string{"A", "B"}, "normalized_score >", []any{p33}},
		{[]string{"A", "C"}, "normalized_score <=", []any{p33, p66}},
		{[]string{"B", "C"}, "normalized_score <=", []any{p66}},
		{[]string{"A", "B", "C"}, "", nil},
	}
	for _, tt := range tests {
		sql, args, err := smartTargetingPerTagScorePredicate(tt.classes, bounds)
		if err != nil {
			t.Fatalf("classes %v failed: %v", tt.classes, err)
		}
		if !strings.Contains(sql, tt.wantSQL) || !reflect.DeepEqual(args, tt.wantArgs) {
			t.Fatalf("classes %v predicate = %q %#v, want fragment %q args %#v", tt.classes, sql, args, tt.wantSQL, tt.wantArgs)
		}
	}

	emptyBounds := &SmartTargetingScoreBounds{}
	if sql, args, err := smartTargetingPerTagScorePredicate([]string{"A"}, emptyBounds); err != nil || !strings.Contains(sql, "AND FALSE") || len(args) != 0 {
		t.Fatalf("empty population predicate = %q %#v, %v; want AND FALSE", sql, args, err)
	}
	if _, _, err := smartTargetingPerTagScorePredicate([]string{"A"}, nil); err == nil {
		t.Fatal("restricted classes without score bounds must fail closed")
	}
}
