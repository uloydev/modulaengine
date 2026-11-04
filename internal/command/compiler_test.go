package command_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/uloydev/modulaengine/internal/command"
	"github.com/uloydev/modulaengine/internal/core"
	"github.com/uloydev/modulaengine/internal/query"
)

func createTestOrderCollection(t *testing.T) *core.Collection {
	t.Helper()

	col, err := core.NewCollection(1, "orders", "Orders")
	if err != nil {
		t.Fatalf("unexpected error creating collection: %v", err)
	}

	precision := 14
	scale := 2

	fields := []*core.Field{
		{Name: "order_number", Type: core.FieldTypeText, IsRequired: true, IsSearchable: true},
		{Name: "total_amount", Type: core.FieldTypeNumber, Precision: &precision, Scale: &scale, IsRequired: true},
		{Name: "status", Type: core.FieldTypeEnum, Options: []string{"DRAFT", "PAID", "CANCELLED"}},
		{Name: "secret_discount", Type: core.FieldTypeNumber},
	}

	for _, f := range fields {
		if err := col.AddField(f); err != nil {
			t.Fatalf("unexpected error adding field '%s': %v", f.Name, err)
		}
	}

	return col
}

func TestCompiler_CompileInsert(t *testing.T) {
	t.Parallel()

	compiler := command.NewCompiler()
	col := createTestOrderCollection(t)

	secCtx := &core.SecurityContext{
		TenantID:      1,
		UserID:        42,
		AllowedFields: []string{"order_number", "total_amount", "status"},
	}

	t.Run("valid insert", func(t *testing.T) {
		t.Parallel()
		data := map[string]any{
			"order_number": "ORD-001",
			"total_amount": 150000.0,
			"status":       "DRAFT",
		}

		mutation, err := compiler.CompileInsert(col, data, secCtx)
		if err != nil {
			t.Fatalf("CompileInsert() error = %v", err)
		}

		if !strings.Contains(mutation.SQL, "INSERT INTO usr_orders") {
			t.Errorf("expected INSERT INTO usr_orders, got: %s", mutation.SQL)
		}
		if !strings.Contains(mutation.SQL, "RETURNING id, public_id, version, created_at, updated_at;") {
			t.Errorf("missing RETURNING clause in SQL: %s", mutation.SQL)
		}
		if len(mutation.FieldDiff) != 3 {
			t.Errorf("expected 3 field diffs, got %d", len(mutation.FieldDiff))
		}
	})

	t.Run("missing required field", func(t *testing.T) {
		t.Parallel()
		data := map[string]any{
			"order_number": "ORD-001",
			// missing total_amount
		}

		_, err := compiler.CompileInsert(col, data, secCtx)
		if !errors.Is(err, core.ErrValidation) {
			t.Fatalf("expected ErrValidation for missing required field, got %v", err)
		}
	})

	t.Run("nil required field", func(t *testing.T) {
		t.Parallel()
		data := map[string]any{
			"order_number": nil,
			"total_amount": 100.0,
		}

		_, err := compiler.CompileInsert(col, data, secCtx)
		if !errors.Is(err, core.ErrValidation) {
			t.Fatalf("expected ErrValidation for nil required field, got %v", err)
		}
	})

	t.Run("empty required text field", func(t *testing.T) {
		t.Parallel()
		data := map[string]any{
			"order_number": "   ",
			"total_amount": 100.0,
		}

		_, err := compiler.CompileInsert(col, data, secCtx)
		if !errors.Is(err, core.ErrValidation) {
			t.Fatalf("expected ErrValidation for empty required text, got %v", err)
		}
	})

	t.Run("forbidden field write", func(t *testing.T) {
		t.Parallel()
		data := map[string]any{
			"order_number":    "ORD-002",
			"total_amount":    200.0,
			"secret_discount": 50.0, // forbidden for secCtx
		}

		_, err := compiler.CompileInsert(col, data, secCtx)
		if !errors.Is(err, query.ErrFieldNotPermitted) {
			t.Fatalf("expected ErrFieldNotPermitted, got %v", err)
		}
	})

	t.Run("invalid enum option", func(t *testing.T) {
		t.Parallel()
		data := map[string]any{
			"order_number": "ORD-003",
			"total_amount": 500.0,
			"status":       "BOGUS_STATUS",
		}

		_, err := compiler.CompileInsert(col, data, secCtx)
		if !errors.Is(err, core.ErrValidation) {
			t.Fatalf("expected ErrValidation for invalid enum option, got %v", err)
		}
	})
}

func TestCompiler_CompileUpdate_OptimisticLocking(t *testing.T) {
	t.Parallel()

	compiler := command.NewCompiler()
	col := createTestOrderCollection(t)

	secCtx := &core.SecurityContext{
		TenantID:      1,
		UserID:        42,
		AllowedFields: []string{"status", "total_amount"},
	}

	t.Run("valid update with OCC check", func(t *testing.T) {
		t.Parallel()
		publicID := "018e32c8-8869-7c91-b3fa-e866184a1d52"
		data := map[string]any{
			"status": "PAID",
		}

		mutation, err := compiler.CompileUpdate(col, publicID, 1, data, secCtx)
		if err != nil {
			t.Fatalf("CompileUpdate() error = %v", err)
		}

		// Verify version increment
		if !strings.Contains(mutation.SQL, "version = version + 1") {
			t.Errorf("missing version increment in SQL: %s", mutation.SQL)
		}
		// Verify optimistic concurrency check in WHERE clause
		if !strings.Contains(mutation.SQL, "WHERE public_id = $") || !strings.Contains(mutation.SQL, "AND version = $") {
			t.Errorf("missing OCC WHERE clause: %s", mutation.SQL)
		}
		// Verify tenant_id check
		if !strings.Contains(mutation.SQL, "AND tenant_id = $") {
			t.Errorf("missing tenant_id check: %s", mutation.SQL)
		}
		// Verify deleted_at check
		if !strings.Contains(mutation.SQL, "AND deleted_at IS NULL") {
			t.Errorf("missing deleted_at IS NULL check: %s", mutation.SQL)
		}
	})

	t.Run("prevent mutating system column", func(t *testing.T) {
		t.Parallel()
		publicID := "018e32c8-8869-7c91-b3fa-e866184a1d52"
		data := map[string]any{
			"id": int64(999),
		}

		_, err := compiler.CompileUpdate(col, publicID, 1, data, secCtx)
		if !errors.Is(err, core.ErrValidation) {
			t.Fatalf("expected ErrValidation for mutating system column, got %v", err)
		}
	})
}

func TestCompiler_CompileSoftDelete(t *testing.T) {
	t.Parallel()

	compiler := command.NewCompiler()
	col := createTestOrderCollection(t)

	secCtx := &core.SecurityContext{
		TenantID: 1,
		UserID:   42,
	}

	publicID := "018e32c8-8869-7c91-b3fa-e866184a1d52"
	mutation, err := compiler.CompileSoftDelete(col, publicID, 2, secCtx)
	if err != nil {
		t.Fatalf("CompileSoftDelete() error = %v", err)
	}

	if !strings.Contains(mutation.SQL, "deleted_at = now()") {
		t.Errorf("missing deleted_at = now() in SQL: %s", mutation.SQL)
	}
	if !strings.Contains(mutation.SQL, "version = version + 1") {
		t.Errorf("missing version = version + 1 in SQL: %s", mutation.SQL)
	}
	if !strings.Contains(mutation.SQL, "WHERE public_id = $") || !strings.Contains(mutation.SQL, "AND version = $") {
		t.Errorf("missing OCC WHERE clause in soft delete: %s", mutation.SQL)
	}
}
