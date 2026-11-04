package integration_test

import (
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uloydev/modulaengine/internal/command"
	"github.com/uloydev/modulaengine/internal/core"
	"github.com/uloydev/modulaengine/internal/database"
	"github.com/uloydev/modulaengine/internal/query"
)

func TestDogfooding_CRM_TemplateValidation(t *testing.T) {
	t.Parallel()

	manifestPath := filepath.Join("..", "..", "templates", "crm.json")
	manifest, err := core.LoadManifestFile(manifestPath)
	if err != nil {
		t.Fatalf("failed to load crm.json: %v", err)
	}

	tenantID := int64(100)
	cols, err := manifest.BuildCollections(tenantID)
	if err != nil {
		t.Fatalf("BuildCollections error: %v", err)
	}

	ddlCompiler := database.NewDDLCompiler()
	registry := query.NewRegistry()
	queryCompiler := query.NewCompiler(registry)
	cmdCompiler := command.NewCompiler()

	colMap := make(map[string]*core.Collection)
	for _, c := range cols {
		colMap[c.Name] = c
		if err := registry.RegisterCollection(c); err != nil {
			t.Fatalf("RegisterCollection failed: %v", err)
		}

		// 1. Verify Dynamic DDL Compilation for each collection
		ddlStmts, err := ddlCompiler.CompileCreateTable(c)
		if err != nil {
			t.Fatalf("CompileCreateTable failed for %s: %v", c.Name, err)
		}
		ddlSQL := strings.Join(ddlStmts, "\n")

		if !strings.Contains(ddlSQL, "SET LOCAL lock_timeout = '2s';") {
			t.Errorf("missing lock timeout in %s DDL", c.Name)
		}
		if !strings.Contains(ddlSQL, "tenant_id BIGINT NOT NULL") {
			t.Errorf("missing tenant_id column in %s DDL", c.Name)
		}
		if !strings.Contains(ddlSQL, "WHERE deleted_at IS NULL") {
			t.Errorf("missing soft-delete partial index in %s DDL", c.Name)
		}
	}

	// 2. Verify Composite FK constraints on CRM relations
	contactsDDL, _ := ddlCompiler.CompileCreateTable(colMap["contacts"])
	if !strings.Contains(strings.Join(contactsDDL, "\n"), "REFERENCES usr_accounts(tenant_id, id) ON DELETE SET NULL") {
		t.Errorf("contacts.account_id missing composite FK to accounts with SET NULL")
	}

	dealsDDL, _ := ddlCompiler.CompileCreateTable(colMap["deals"])
	dealsSQL := strings.Join(dealsDDL, "\n")
	if !strings.Contains(dealsSQL, "REFERENCES usr_accounts(tenant_id, id) ON DELETE RESTRICT") {
		t.Errorf("deals.account_id missing composite FK to accounts with RESTRICT")
	}

	activitiesDDL, _ := ddlCompiler.CompileCreateTable(colMap["activities"])
	if !strings.Contains(strings.Join(activitiesDDL, "\n"), "REFERENCES usr_deals(tenant_id, id) ON DELETE CASCADE") {
		t.Errorf("activities.deal_id missing composite FK to deals with CASCADE")
	}

	// 3. Verify RLS Filter Injection for sales_rep on deals
	salesRepCtx := &core.SecurityContext{
		TenantID:      tenantID,
		UserID:        42,
		RoleName:      "sales_rep",
		AllowedFields: []string{"*"},
		RLSFilter:     `{"op":"eq","field":"created_by","value":"{{user.id}}"}`,
	}

	dealsCol := colMap["deals"]
	compiledSelect, err := queryCompiler.CompileSelect(dealsCol, nil, salesRepCtx)
	if err != nil {
		t.Fatalf("CompileSelect failed: %v", err)
	}

	if !strings.Contains(compiledSelect.SQL, "created_by = $") {
		t.Errorf("sales_rep deals query missing injected RLS created_by = $N: %s", compiledSelect.SQL)
	}

	// 4. Verify Optimistic Concurrency Control (OCC) Mutation on deals
	updateData := map[string]any{"stage": "NEGOTIATION"}
	publicID := "018e32c8-8869-7c91-b3fa-e866184a1d52"
	mutation, err := cmdCompiler.CompileUpdate(dealsCol, publicID, 1, updateData, salesRepCtx)
	if err != nil {
		t.Fatalf("CompileUpdate failed: %v", err)
	}

	if !strings.Contains(mutation.SQL, "version = version + 1") {
		t.Errorf("mutation missing version increment: %s", mutation.SQL)
	}
	if !strings.Contains(mutation.SQL, "WHERE public_id = $") || !strings.Contains(mutation.SQL, "AND version = $") {
		t.Errorf("mutation missing optimistic locking check: %s", mutation.SQL)
	}

	// 5. Verify Field-level Audit Diff
	oldState := map[string]any{"stage": "PROSPECT", "amount": 50000.0}
	newState := map[string]any{"stage": "NEGOTIATION", "amount": 50000.0}
	diff := command.ComputeDiff(oldState, newState)
	if len(diff) != 1 {
		t.Fatalf("expected 1 changed field in diff, got %d", len(diff))
	}
	if diff["stage"].Old != "PROSPECT" || diff["stage"].New != "NEGOTIATION" {
		t.Errorf("unexpected diff for stage: %+v", diff["stage"])
	}
}

func TestDogfooding_Commerce_TemplateValidation(t *testing.T) {
	t.Parallel()

	manifestPath := filepath.Join("..", "..", "templates", "commerce.json")
	manifest, err := core.LoadManifestFile(manifestPath)
	if err != nil {
		t.Fatalf("failed to load commerce.json: %v", err)
	}

	tenantID := int64(200)
	cols, err := manifest.BuildCollections(tenantID)
	if err != nil {
		t.Fatalf("BuildCollections error: %v", err)
	}

	ddlCompiler := database.NewDDLCompiler()
	registry := query.NewRegistry()
	queryCompiler := query.NewCompiler(registry)

	colMap := make(map[string]*core.Collection)
	for _, c := range cols {
		colMap[c.Name] = c
		if err := registry.RegisterCollection(c); err != nil {
			t.Fatalf("RegisterCollection failed: %v", err)
		}
	}

	// 1. Verify Self-referential Relation (categories.parent_id -> categories)
	catDDL, err := ddlCompiler.CompileCreateTable(colMap["categories"])
	if err != nil {
		t.Fatalf("categories DDL failed: %v", err)
	}
	if !strings.Contains(strings.Join(catDDL, "\n"), "REFERENCES usr_categories(tenant_id, id) ON DELETE SET NULL") {
		t.Errorf("categories.parent_id missing self-referential composite FK")
	}

	// 2. Verify Cascade on Delete (order_items.order_id -> orders)
	itemsDDL, err := ddlCompiler.CompileCreateTable(colMap["order_items"])
	if err != nil {
		t.Fatalf("order_items DDL failed: %v", err)
	}
	if !strings.Contains(strings.Join(itemsDDL, "\n"), "REFERENCES usr_orders(tenant_id, id) ON DELETE CASCADE") {
		t.Errorf("order_items.order_id missing composite FK to orders with CASCADE")
	}

	// 3. Verify FLS Field Masking on Storefront: cost_price MUST NOT leak
	storefrontCtx := &core.SecurityContext{
		TenantID:      tenantID,
		UserID:        0, // anonymous
		RoleName:      "public_storefront",
		AllowedFields: []string{"title", "slug", "description", "price", "stock_quantity", "images", "category_id"},
		RLSFilter:     `{"op":"eq","field":"is_published","value":true}`,
	}

	productsCol := colMap["products"]

	// Test default projection omits cost_price
	defaultSelect, err := queryCompiler.CompileSelect(productsCol, nil, storefrontCtx)
	if err != nil {
		t.Fatalf("CompileSelect default failed: %v", err)
	}
	if strings.Contains(defaultSelect.SQL, "cost_price") {
		t.Fatalf("CRITICAL SECURITY LEAK: cost_price leaked in storefront projection: %s", defaultSelect.SQL)
	}

	// Test explicit request for cost_price is rejected with ErrFieldNotPermitted
	reqFields, _ := url.ParseQuery("fields=title,price,cost_price")
	params, _ := query.ParseQueryParams(reqFields)
	_, err = queryCompiler.CompileSelect(productsCol, params, storefrontCtx)
	if !errors.Is(err, query.ErrFieldNotPermitted) {
		t.Fatalf("expected ErrFieldNotPermitted when requesting cost_price, got: %v", err)
	}

	// 4. Verify Anonymous Public RLS Filter: is_published == true injected
	if !strings.Contains(defaultSelect.SQL, "is_published = $") {
		t.Errorf("missing RLS is_published filter in storefront catalog query: %s", defaultSelect.SQL)
	}
}
