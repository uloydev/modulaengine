package core_test

import (
	"path/filepath"
	"testing"

	"github.com/uloydev/modulaengine/internal/core"
)

func TestLoadManifest_CRM(t *testing.T) {
	t.Parallel()

	crmPath := filepath.Join("..", "..", "templates", "crm.json")
	manifest, err := core.LoadManifestFile(crmPath)
	if err != nil {
		t.Fatalf("failed to load crm.json: %v", err)
	}

	if manifest.Name != "b2b_crm" {
		t.Errorf("manifest name = %s, want b2b_crm", manifest.Name)
	}

	cols, err := manifest.BuildCollections(10)
	if err != nil {
		t.Fatalf("failed to build collections: %v", err)
	}

	if len(cols) != 4 {
		t.Fatalf("expected 4 collections, got %d", len(cols))
	}

	colNames := make(map[string]*core.Collection)
	for _, c := range cols {
		colNames[c.Name] = c
	}

	expectedCols := []string{"accounts", "contacts", "deals", "activities"}
	for _, name := range expectedCols {
		if _, exists := colNames[name]; !exists {
			t.Errorf("expected collection '%s' in CRM manifest", name)
		}
	}

	// Verify relations
	dealsCol := colNames["deals"]
	accountRel := dealsCol.Fields["account_id"]
	if accountRel == nil || accountRel.Relation == nil {
		t.Fatalf("expected account_id relation on deals")
	}
	if accountRel.Relation.TargetCollection != "accounts" {
		t.Errorf("expected target accounts, got %s", accountRel.Relation.TargetCollection)
	}

	// Verify permissions
	perms, err := manifest.BuildPermissions(10, cols)
	if err != nil {
		t.Fatalf("failed to build permissions: %v", err)
	}
	if len(perms) != 3 {
		t.Errorf("expected 3 permissions for sales_rep, got %d", len(perms))
	}
}

func TestLoadManifest_Commerce(t *testing.T) {
	t.Parallel()

	commercePath := filepath.Join("..", "..", "templates", "commerce.json")
	manifest, err := core.LoadManifestFile(commercePath)
	if err != nil {
		t.Fatalf("failed to load commerce.json: %v", err)
	}

	if manifest.Name != "headless_commerce" {
		t.Errorf("manifest name = %s, want headless_commerce", manifest.Name)
	}

	cols, err := manifest.BuildCollections(20)
	if err != nil {
		t.Fatalf("failed to build collections: %v", err)
	}

	if len(cols) != 4 {
		t.Fatalf("expected 4 collections, got %d", len(cols))
	}

	colNames := make(map[string]*core.Collection)
	for _, c := range cols {
		colNames[c.Name] = c
	}

	// Verify self-referential categories.parent_id -> categories
	catCol := colNames["categories"]
	parentRel := catCol.Fields["parent_id"]
	if parentRel == nil || parentRel.Relation == nil {
		t.Fatalf("expected parent_id relation on categories")
	}
	if parentRel.Relation.TargetCollection != "categories" {
		t.Errorf("expected self-referential target categories, got %s", parentRel.Relation.TargetCollection)
	}

	// Verify cascade on delete on order_items.order_id
	itemsCol := colNames["order_items"]
	orderRel := itemsCol.Fields["order_id"]
	if orderRel == nil || orderRel.Relation == nil {
		t.Fatalf("expected order_id relation on order_items")
	}
	if orderRel.Relation.OnDelete != core.OnDeleteCascade {
		t.Errorf("expected CASCADE on delete, got %s", orderRel.Relation.OnDelete)
	}
}
