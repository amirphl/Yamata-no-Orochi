package scheduler

import (
	"reflect"
	"testing"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/lib/pq"
)

func TestAssignFirstMatchingTagsUsesSelectionOrderWithoutReorderingAudience(t *testing.T) {
	rows := []*models.AudienceProfile{
		{ID: 101, Tags: pq.Int32Array{5, 9}},
		{ID: 102, Tags: pq.Int32Array{5}},
		{ID: 103, Tags: pq.Int32Array{2, 9}},
	}
	beforeIDs := []int64{rows[0].ID, rows[1].ID, rows[2].ID}

	assigned := assignFirstMatchingTags(rows, []int64{9, 5, 2})

	if want := []uint{9, 5, 9}; !reflect.DeepEqual(assigned, want) {
		t.Fatalf("assigned tags = %v, want %v", assigned, want)
	}
	afterIDs := []int64{rows[0].ID, rows[1].ID, rows[2].ID}
	if !reflect.DeepEqual(afterIDs, beforeIDs) {
		t.Fatalf("tag attribution changed score-priority order from %v to %v", beforeIDs, afterIDs)
	}
}

func TestAssignFirstMatchingTagsLeavesMissingAttributionExplicit(t *testing.T) {
	assigned := assignFirstMatchingTags(
		[]*models.AudienceProfile{{ID: 101, Tags: pq.Int32Array{7}}, nil},
		[]int64{9, 5},
	)
	if want := []uint{0, 0}; !reflect.DeepEqual(assigned, want) {
		t.Fatalf("missing assigned tags = %v, want %v", assigned, want)
	}
}
