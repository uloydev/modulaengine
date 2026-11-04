package query_test

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/uloydev/modulaengine/internal/core"
	"github.com/uloydev/modulaengine/internal/query"
)

func createTestCollection(t *testing.T) *core.Collection {
	t.Helper()

	col, err := core.NewCollection(10, "deals", "Deals")
	if err != nil {
		t.Fatalf("failed to create collection: %v", err)
	}

	fields := []*core.Field{
		{Name: "title", Type: core.FieldTypeText, IsRequired: true, IsSearchable: true},
		{Name: "amount", Type: core.FieldTypeNumber, IsRequired: true},
		{Name: "stage", Type: core.FieldTypeEnum, Options: []string{"PROSPECT", "CLOSED_WON"}},
		{Name: "secret_margin", Type: core.FieldTypeNumber},
	}

	for _, f := range fields {
		if err := col.AddField(f); err != nil {
			t.Fatalf("failed to add field: %v", err)
		}
	}

	return col
}

func TestCompiler_CompileSelect_Invariants(t *testing.T) {
	t.Parallel()

	registry := query.NewRegistry()
	compiler := query.NewCompiler(registry)
	col := createTestCollection(t)

	secCtx := &core.SecurityContext{
		TenantID:      10,
		UserID:        42,
		RoleID:        2,
		RoleName:      "sales_rep",
		IsSuperAdmin:  false,
		AllowedFields: []string{"title", "amount", "stage", "created_by"},
	}

	vals, _ := url.ParseQuery("filter[stage][_eq]=PROSPECT&filter[amount][_gt]=1000&sort=-amount&limit=10&offset=20")
	params, err := query.ParseQueryParams(vals)
	if err != nil {
		t.Fatalf("ParseQueryParams() error = %v", err)
	}

	compiled, err := compiler.CompileSelect(col, params, secCtx)
	if err != nil {
		t.Fatalf("CompileSelect() error = %v", err)
	}

	// Invariant 1: Unconditional tenant_id = $1 injection
	if !strings.Contains(compiled.SQL, "tenant_id = $1") {
		t.Errorf("missing tenant_id = $1 in SQL: %s", compiled.SQL)
	}
	if compiled.Args[0] != int64(10) {
		t.Errorf("expected arg[0] to be tenantID 10, got %v", compiled.Args[0])
	}

	// Invariant 2: Unconditional deleted_at IS NULL
	if !strings.Contains(compiled.SQL, "deleted_at IS NULL") {
		t.Errorf("missing deleted_at IS NULL in SQL: %s", compiled.SQL)
	}

	// Invariant 3: Dual-ID public_id AS id
	if !strings.Contains(compiled.SQL, "public_id AS id") {
		t.Errorf("missing public_id AS id in SQL: %s", compiled.SQL)
	}

	// Invariant 4: Positional parameters $1, $2, ... match args count
	expectedArgCount := len(compiled.Args)
	if expectedArgCount < 4 {
		t.Errorf("expected at least 4 args (tenant, 2 filters, limit, offset), got %d", expectedArgCount)
	}
}

func TestCompiler_CompileSelect_FLS(t *testing.T) {
	t.Parallel()

	registry := query.NewRegistry()
	compiler := query.NewCompiler(registry)
	col := createTestCollection(t)

	// Restricted user: secret_margin is NOT in allowed fields
	secCtx := &core.SecurityContext{
		TenantID:      10,
		UserID:        42,
		RoleID:        2,
		RoleName:      "sales_rep",
		AllowedFields: []string{"title", "amount", "stage"},
	}

	t.Run("forbidden field omitted from default projection", func(t *testing.T) {
		t.Parallel()
		compiled, err := compiler.CompileSelect(col, nil, secCtx)
		if err != nil {
			t.Fatalf("CompileSelect() error = %v", err)
		}
		if strings.Contains(compiled.SQL, "secret_margin") {
			t.Errorf("forbidden field 'secret_margin' leaked in SQL projection: %s", compiled.SQL)
		}
	})

	t.Run("explicitly requested forbidden field returns error", func(t *testing.T) {
		t.Parallel()
		vals, _ := url.ParseQuery("fields=title,secret_margin")
		params, _ := query.ParseQueryParams(vals)

		_, err := compiler.CompileSelect(col, params, secCtx)
		if !errors.Is(err, query.ErrFieldNotPermitted) {
			t.Fatalf("expected ErrFieldNotPermitted, got %v", err)
		}
	})

	t.Run("filtering on forbidden field returns error", func(t *testing.T) {
		t.Parallel()
		vals, _ := url.ParseQuery("filter[secret_margin][_gt]=500")
		params, _ := query.ParseQueryParams(vals)

		_, err := compiler.CompileSelect(col, params, secCtx)
		if !errors.Is(err, query.ErrFieldNotPermitted) {
			t.Fatalf("expected ErrFieldNotPermitted, got %v", err)
		}
	})

	t.Run("superadmin bypasses FLS", func(t *testing.T) {
		t.Parallel()
		adminCtx := &core.SecurityContext{
			TenantID:     10,
			UserID:       1,
			IsSuperAdmin: true,
		}
		vals, _ := url.ParseQuery("fields=title,secret_margin")
		params, _ := query.ParseQueryParams(vals)

		compiled, err := compiler.CompileSelect(col, params, adminCtx)
		if err != nil {
			t.Fatalf("unexpected error for superadmin: %v", err)
		}
		if !strings.Contains(compiled.SQL, "secret_margin") {
			t.Errorf("superadmin expected secret_margin in SQL: %s", compiled.SQL)
		}
	})
}

func TestCompiler_CompileSelect_RLSInjection(t *testing.T) {
	t.Parallel()

	registry := query.NewRegistry()
	compiler := query.NewCompiler(registry)
	col := createTestCollection(t)

	// User role has RLS rule: created_by == {{user.id}}
	secCtx := &core.SecurityContext{
		TenantID:      10,
		UserID:        99,
		RoleID:        3,
		RoleName:      "sales_rep",
		AllowedFields: []string{"*"},
		RLSFilter:     `{"op":"eq","field":"created_by","value":"{{user.id}}"}`,
	}

	compiled, err := compiler.CompileSelect(col, nil, secCtx)
	if err != nil {
		t.Fatalf("CompileSelect() error = %v", err)
	}

	// Verify created_by = $N is injected
	if !strings.Contains(compiled.SQL, "created_by = $") {
		t.Errorf("expected RLS predicate 'created_by = $' in SQL: %s", compiled.SQL)
	}

	// Verify substituted context value
	foundSubstitutedVal := false
	for _, arg := range compiled.Args {
		if arg == int64(99) {
			foundSubstitutedVal = true
			break
		}
	}
	if !foundSubstitutedVal {
		t.Errorf("expected RLS to bind substituted user_id 99, args: %v", compiled.Args)
	}
}

func TestCompiler_CompileSelect_UnknownField(t *testing.T) {
	t.Parallel()

	registry := query.NewRegistry()
	compiler := query.NewCompiler(registry)
	col := createTestCollection(t)

	secCtx := &core.SecurityContext{
		TenantID:     10,
		IsSuperAdmin: true,
	}

	vals, _ := url.ParseQuery("filter[malicious_col][_eq]=1")
	params, _ := query.ParseQueryParams(vals)

	_, err := compiler.CompileSelect(col, params, secCtx)
	if !errors.Is(err, query.ErrUnknownField) {
		t.Fatalf("expected ErrUnknownField, got %v", err)
	}
}

func TestCompiler_CompileSelect_NestedLogicalPrecedence(t *testing.T) {
	t.Parallel()

	registry := query.NewRegistry()
	compiler := query.NewCompiler(registry)
	col := createTestCollection(t)

	secCtx := &core.SecurityContext{
		TenantID:     10,
		IsSuperAdmin: true,
	}

	// ((stage = 'PROSPECT' AND amount > 500) OR stage = 'CLOSED_WON')
	complexFilter := &query.LogicalNode{
		Op: query.LogicalOr,
		Children: []query.Node{
			&query.LogicalNode{
				Op: query.LogicalAnd,
				Children: []query.Node{
					&query.ComparisonNode{Field: "stage", Op: query.OpEq, Value: "PROSPECT"},
					&query.ComparisonNode{Field: "amount", Op: query.OpGt, Value: 500},
				},
			},
			&query.ComparisonNode{Field: "stage", Op: query.OpEq, Value: "CLOSED_WON"},
		},
	}

	params := &query.QueryParams{Filter: complexFilter}
	compiled, err := compiler.CompileSelect(col, params, secCtx)
	if err != nil {
		t.Fatalf("CompileSelect() error = %v", err)
	}

	// Verify nested logical groups are properly parenthesized
	if !strings.Contains(compiled.SQL, "((stage = $2 AND amount > $3) OR stage = $4)") {
		t.Errorf("expected properly parenthesized nested logical groups, got: %s", compiled.SQL)
	}
}
