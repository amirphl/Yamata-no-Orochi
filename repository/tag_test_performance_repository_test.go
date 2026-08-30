package repository

import (
	"strings"
	"testing"
)

func TestRecomputeCampaignTagPerformanceSQLUsesSingleAudienceAttribution(t *testing.T) {
	for _, fragment := range []string{
		"SELECT DISTINCT ON (attribution.campaign_id, attribution.audience_id)",
		"attribution.assigned_tag_id AS tag_id",
		"source_campaign.bundle_id = attribution.bundle_id",
		"sent.bundle_audience_selection_id = attributed.bundle_audience_selection_id",
		"delivered.bundle_audience_selection_id = attributed.bundle_audience_selection_id",
		"status.total_parts > 0",
		"status.total_parts = status.total_delivered_parts",
		"SELECT DISTINCT attributed.audience_id",
		"click.phone_number = attributed.phone_number",
		"click.uid IS NOT NULL",
		"ON CONFLICT (campaign_id, tag_id) DO UPDATE",
	} {
		if !strings.Contains(recomputeCampaignTagPerformanceSQL, fragment) {
			t.Fatalf("tag performance query does not contain %q", fragment)
		}
	}
	if strings.Contains(recomputeCampaignTagPerformanceSQL, "FOR UPDATE") {
		t.Fatal("tag performance query must not lock click or short-link rows")
	}
}

func TestClaimPendingTagTestReportsSQLDoesNotRetryTerminalFailures(t *testing.T) {
	if !strings.Contains(claimPendingTagTestReportsSQL, "calculation_version = ?") {
		t.Fatal("claim query does not isolate workers by calculation version")
	}
	if !strings.Contains(claimPendingTagTestReportsSQL, "status = 'failed' AND next_retry_at <= ?") {
		t.Fatal("claim query does not require a scheduled retry time for failed reports")
	}
	if strings.Contains(claimPendingTagTestReportsSQL, "next_retry_at IS NULL OR") {
		t.Fatal("claim query retries terminal failed reports")
	}
	if got, want := strings.Count(claimPendingTagTestReportsSQL, "?"), 6; got != want {
		t.Fatalf("claim query bind count = %d, want %d", got, want)
	}
}

func TestDiscoverPendingTagTestReportsSQLBindsSourcesAndCalculationVersion(t *testing.T) {
	if !strings.Contains(discoverPendingTagTestReportsSQL, "existing.calculation_version <> ?") {
		t.Fatal("discovery query does not enqueue stale calculation versions")
	}
	if got, want := strings.Count(discoverPendingTagTestReportsSQL, "?"), 14; got != want {
		t.Fatalf("discovery query bind count = %d, want %d", got, want)
	}
}

func TestSummarySQLUsesWeightedTotalsAndStableUpsert(t *testing.T) {
	for _, fragment := range []string{
		"SUM(performance.delivered_count)",
		"SUM(performance.click_count)",
		"ON CONFLICT (bundle_id, tag_id) DO UPDATE",
	} {
		if !strings.Contains(summarySQL, fragment) {
			t.Fatalf("summary query does not contain %q", fragment)
		}
	}
}

func TestCampaignMaterializationDeletesTagsNoLongerAttributed(t *testing.T) {
	for _, fragment := range []string{
		"DELETE FROM campaign_tag_test_performances",
		"attribution.bundle_id = performance.bundle_id",
		"attribution.assigned_tag_id = performance.tag_id",
	} {
		if !strings.Contains(deleteStaleCampaignTagPerformancesSQL, fragment) {
			t.Fatalf("stale Campaign materialization query does not contain %q", fragment)
		}
	}
}

func TestRecomputeCampaignTagPerformanceSQLBindsEverySource(t *testing.T) {
	// Campaign + phase, four send sources, four delivery sources, and three
	// materialization metadata values.
	if got, want := strings.Count(recomputeCampaignTagPerformanceSQL, "?"), 13; got != want {
		t.Fatalf("tag performance query bind count = %d, want %d", got, want)
	}
}
