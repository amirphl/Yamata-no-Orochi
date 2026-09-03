package repository

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestCampaignTargetingTestSamplingCompleteQueryGuardsLeaseAndBindsExactValues(t *testing.T) {
	db := newAudienceProfileDryRunDB(t).Session(&gorm.Session{SkipDefaultTransaction: true})
	repo := &CampaignTargetingTestSamplingRepositoryImpl{db: db}
	lease := time.Date(2026, time.August, 16, 10, 0, 0, 0, time.UTC)
	finished := lease.Add(time.Minute)
	results := json.RawMessage(`[{"tag_id":9,"selection_order":0,"satisfied":true,"available_count":600}]`)
	fingerprint := strings.Repeat("a", 64)

	statement := repo.completeQuery(context.Background(), 41, lease, results, 1, 600, math.MaxUint64, fingerprint, finished).Statement
	if statement.Error != nil {
		t.Fatalf("build sampling completion query: %v", statement.Error)
	}
	sql := statement.SQL.String()
	for _, fragment := range []string{"status", "started_at", "::jsonb", "::numeric", "allocation_fingerprint"} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("sampling completion query does not contain %q:\n%s", fragment, sql)
		}
	}
	foundMaxCost := false
	for _, variable := range statement.Vars {
		if value, ok := variable.(string); ok && value == "18446744073709551615" {
			foundMaxCost = true
			break
		}
	}
	if !foundMaxCost {
		t.Fatalf("sampling completion query does not bind MaxUint64 as an exact decimal: %#v", statement.Vars)
	}
	foundFingerprint := false
	for _, variable := range statement.Vars {
		if value, ok := variable.(string); ok && value == fingerprint {
			foundFingerprint = true
			break
		}
	}
	if !foundFingerprint {
		t.Fatalf("sampling completion query does not bind allocation fingerprint: %#v", statement.Vars)
	}
}

func TestCampaignTargetingTestSamplingCompleteRejectsInvalidAllocationFingerprint(t *testing.T) {
	db := newAudienceProfileDryRunDB(t).Session(&gorm.Session{SkipDefaultTransaction: true})
	repo := &CampaignTargetingTestSamplingRepositoryImpl{db: db}
	now := time.Now().UTC()
	for _, fingerprint := range []string{
		"not-a-sha256-fingerprint",
		strings.Repeat("z", 64),
		strings.Repeat("A", 64),
	} {
		if err := repo.Complete(context.Background(), 41, now, json.RawMessage(`[]`), 0, 0, 0, fingerprint, now); err == nil {
			t.Fatalf("Complete accepted invalid allocation fingerprint %q", fingerprint)
		}
	}
}
