package command_test

import (
	"reflect"
	"testing"

	"github.com/uloydev/modulaengine/internal/command"
)

func TestComputeDiff(t *testing.T) {
	t.Parallel()

	oldVals := map[string]any{
		"status": "DRAFT",
		"amount": 100.0,
		"notes":  "to be removed",
	}

	newVals := map[string]any{
		"status": "PAID",
		"amount": 100.0, // unchanged
		"tag":    "urgent",
	}

	diff := command.ComputeDiff(oldVals, newVals)

	// status changed
	statusDiff, exists := diff["status"]
	if !exists {
		t.Fatalf("expected diff for 'status'")
	}
	if statusDiff.Old != "DRAFT" || statusDiff.New != "PAID" {
		t.Errorf("unexpected status diff: %+v", statusDiff)
	}

	// amount unchanged -> should NOT be in diff
	if _, exists := diff["amount"]; exists {
		t.Errorf("amount was unchanged but appeared in diff")
	}

	// notes removed
	notesDiff, exists := diff["notes"]
	if !exists {
		t.Fatalf("expected diff for 'notes'")
	}
	if notesDiff.Old != "to be removed" || notesDiff.New != nil {
		t.Errorf("unexpected notes diff: %+v", notesDiff)
	}

	// tag added
	tagDiff, exists := diff["tag"]
	if !exists {
		t.Fatalf("expected diff for 'tag'")
	}
	if tagDiff.Old != nil || tagDiff.New != "urgent" {
		t.Errorf("unexpected tag diff: %+v", tagDiff)
	}
}

func TestComputeDiff_Identical(t *testing.T) {
	t.Parallel()

	vals := map[string]any{
		"status": "ACTIVE",
		"count":  42,
	}

	diff := command.ComputeDiff(vals, vals)
	if len(diff) != 0 {
		t.Fatalf("expected empty diff for identical maps, got: %+v", diff)
	}
}

func TestComputeDiff_NestedMaps(t *testing.T) {
	t.Parallel()

	oldVals := map[string]any{
		"meta": map[string]any{"color": "red", "size": 10},
	}
	newVals := map[string]any{
		"meta": map[string]any{"color": "blue", "size": 10},
	}

	diff := command.ComputeDiff(oldVals, newVals)
	if len(diff) != 1 {
		t.Fatalf("expected 1 diff entry, got: %d", len(diff))
	}
	metaDiff := diff["meta"]
	if reflect.DeepEqual(metaDiff.Old, metaDiff.New) {
		t.Errorf("expected meta diff to show difference")
	}
}
