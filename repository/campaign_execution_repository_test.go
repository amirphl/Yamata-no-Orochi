package repository

import (
	"strings"
	"testing"
)

func TestBundleCampaignAllocationsExcludeExplicitAndLegacyExcelTargeting(t *testing.T) {
	t.Parallel()

	db := newAudienceProfileDryRunDB(t)
	var rows []BundleCampaignAllocation
	statement := bundleCampaignAllocationsQuery(db, 44, 55).Find(&rows).Statement
	if statement.Error != nil {
		t.Fatalf("build bundle allocation query: %v", statement.Error)
	}
	sql := strings.ToLower(statement.SQL.String())
	for _, required := range []string{
		"audience_targeting_method",
		"target_audience_excel_file_uuid",
		"case",
		"bundle_audience_selections",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("bundle allocation query does not contain %q:\n%s", required, sql)
		}
	}
	if strings.Contains(sql, "then 'excel'") || strings.Contains(sql, "<> 'excel'") {
		t.Fatalf("bundle allocation query interpolated targeting values:\n%s", sql)
	}

	excelArgs := 0
	for _, value := range statement.Vars {
		if text, ok := value.(string); ok && text == "excel" {
			excelArgs++
		}
	}
	if excelArgs != 3 {
		t.Fatalf("Excel targeting bind count = %d, want 3", excelArgs)
	}
}
