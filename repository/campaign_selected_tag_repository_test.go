package repository

import (
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestSmartTagOrderAllowlist(t *testing.T) {
	tests := []struct {
		sortBy    string
		direction string
		want      string
	}{
		{"database_order", "asc", "available_tags.tag_id ASC"},
		{"tag_capacity", "desc", "available_tags.tag_audience_count DESC NULLS LAST, available_tags.tag_id ASC"},
		{"bundle_persona_fit_score", "asc", "available_tags.bundle_persona_fit_score ASC NULLS LAST, available_tags.tag_id ASC"},
		{"test_phase_avg_ctr", "desc", "available_tags.tag_id ASC"},
	}
	for _, tt := range tests {
		got, err := smartTagOrder(tt.sortBy, tt.direction)
		if err != nil || got != tt.want {
			t.Fatalf("smartTagOrder(%q, %q) = %q, %v; want %q", tt.sortBy, tt.direction, got, err, tt.want)
		}
	}
	if _, err := smartTagOrder("tags.id; DROP TABLE tags", "asc"); err == nil {
		t.Fatal("expected arbitrary sort expression to be rejected")
	}
	if _, err := smartTagOrder("tag_capacity", "sideways"); err == nil {
		t.Fatal("expected invalid direction to be rejected")
	}
}

func TestEscapeLike(t *testing.T) {
	if got, want := escapeLike(`50%_off\today`), `50\%\_off\\today`; got != want {
		t.Fatalf("escapeLike() = %q, want %q", got, want)
	}
}

func TestAvailableSmartTagsQueryPrefersScoreSnapshotWithCatalogFallback(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=localhost user=test dbname=test sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}

	var rows []uint
	statement := availableSmartTagsQuery(db, 42).
		Select("available_tags.tag_id").
		Find(&rows).Statement
	if statement.Error != nil {
		t.Fatalf("build available-tag query: %v", statement.Error)
	}
	sql := statement.SQL.String()
	for _, fragment := range []string{
		"FROM current_bundle_tag_scores AS scores",
		"JOIN tags AS scored_tags",
		"scored_tags.is_active = TRUE",
		"scores.tag_name_snapshot",
		"scores.tag_persona_snapshot",
		"scores.tag_audience_count_snapshot",
		"UNION ALL",
		"FROM tags",
		"NOT EXISTS",
		"FROM current_bundle_tag_scores AS existing_scores",
		"existing_tags.is_active = TRUE",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("available-tag query does not contain %q:\n%s", fragment, sql)
		}
	}
	if got, want := len(statement.Vars), 2; got != want {
		t.Fatalf("bundle bind count = %d, want %d", got, want)
	}
	for i, variable := range statement.Vars {
		if variable != uint(42) {
			t.Fatalf("bundle bind %d = %#v, want 42", i, variable)
		}
	}
}
