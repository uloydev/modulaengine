package database_test

import (
	"strings"
	"testing"

	"github.com/uloydev/modulaengine/internal/core"
	"github.com/uloydev/modulaengine/internal/database"
)

func TestDDLCompiler_CompileCreateTable(t *testing.T) {
	t.Parallel()

	compiler := database.NewDDLCompiler()

	col, err := core.NewCollection(1, "orders", "Orders")
	if err != nil {
		t.Fatalf("unexpected error creating collection: %v", err)
	}

	precision := 14
	scale := 2

	fields := []*core.Field{
		{
			Name:         "order_number",
			Type:         core.FieldTypeText,
			IsRequired:   true,
			IsSearchable: true,
		},
		{
			Name:      "total_amount",
			Type:      core.FieldTypeNumber,
			Precision: &precision,
			Scale:     &scale,
		},
		{
			Name:    "status",
			Type:    core.FieldTypeEnum,
			Options: []string{"DRAFT", "PAID", "CANCELLED"},
		},
		{
			Name: "customer_id",
			Type: core.FieldTypeRelation,
			Relation: &core.RelationDef{
				RelationType:     "M2O",
				TargetCollection: "customers",
				OnDelete:         core.OnDeleteRestrict,
			},
		},
		{
			Name: "metadata",
			Type: core.FieldTypeJSON,
		},
	}

	for _, f := range fields {
		if err := col.AddField(f); err != nil {
			t.Fatalf("unexpected error adding field '%s': %v", f.Name, err)
		}
	}

	stmts, err := compiler.CompileCreateTable(col)
	if err != nil {
		t.Fatalf("CompileCreateTable() error = %v", err)
	}

	allSQL := strings.Join(stmts, "\n")

	invariants := []struct {
		name     string
		expected string
	}{
		{name: "lock timeout", expected: "SET LOCAL lock_timeout = '2s';"},
		{name: "statement timeout", expected: "SET LOCAL statement_timeout = '5s';"},
		{name: "table name", expected: "CREATE TABLE usr_orders"},
		{name: "internal id", expected: "id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY"},
		{name: "public id", expected: "public_id UUID NOT NULL DEFAULT gen_random_uuid()"},
		{name: "composite tenant pk constraint", expected: "CONSTRAINT uq_orders_tenant_id UNIQUE (tenant_id, id)"},
		{name: "composite tenant public_id constraint", expected: "CONSTRAINT uq_orders_tenant_public_id UNIQUE (tenant_id, public_id)"},
		{name: "composite foreign key", expected: "CONSTRAINT fk_orders_customer_id FOREIGN KEY (tenant_id, customer_id) REFERENCES usr_customers(tenant_id, id) ON DELETE RESTRICT"},
		{name: "enum check constraint", expected: "CONSTRAINT chk_orders_status CHECK (status IN ('DRAFT', 'PAID', 'CANCELLED'))"},
		{name: "numeric precision", expected: "total_amount NUMERIC(14,2)"},
		{name: "active tenant index", expected: "CREATE INDEX idx_orders_tenant_active ON usr_orders(tenant_id) WHERE deleted_at IS NULL;"},
		{name: "public id index", expected: "CREATE INDEX idx_orders_public_lookup ON usr_orders(tenant_id, public_id) WHERE deleted_at IS NULL;"},
		{name: "searchable index", expected: "CREATE INDEX idx_orders_order_number ON usr_orders(tenant_id, order_number) WHERE deleted_at IS NULL;"},
		{name: "fk index", expected: "CREATE INDEX idx_orders_customer_id ON usr_orders(tenant_id, customer_id) WHERE deleted_at IS NULL;"},
	}

	for _, tc := range invariants {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(allSQL, tc.expected) {
				t.Fatalf("generated SQL missing expected invariant: %s\nGenerated SQL:\n%s", tc.expected, allSQL)
			}
		})
	}
}

func TestDDLCompiler_CompileCreateTable_EdgeCases(t *testing.T) {
	t.Parallel()

	compiler := database.NewDDLCompiler()
	col, err := core.NewCollection(1, "accounts", "Accounts")
	if err != nil {
		t.Fatalf("unexpected error creating collection: %v", err)
	}

	boolDefaultTrue := "true"
	fields := []*core.Field{
		{
			Name:         "email",
			Type:         core.FieldTypeText,
			IsUnique:     true,
			IsRequired:   true,
			IsSearchable: true,
		},
		{
			Name:         "is_verified",
			Type:         core.FieldTypeBoolean,
			DefaultValue: &boolDefaultTrue,
		},
		{
			Name:        "api_secret",
			Type:        core.FieldTypeText,
			IsEncrypted: true,
		},
		{
			Name: "parent_account_id",
			Type: core.FieldTypeRelation,
			Relation: &core.RelationDef{
				RelationType:     core.RelationTypeM2O,
				TargetCollection: "accounts",
				OnDelete:         core.OnDeleteSetNull,
			},
		},
		{
			Name: "child_items",
			Type: core.FieldTypeRelation,
			Relation: &core.RelationDef{
				RelationType:     core.RelationTypeO2M,
				TargetCollection: "line_items",
			},
		},
	}

	for _, f := range fields {
		if err := col.AddField(f); err != nil {
			t.Fatalf("unexpected error adding field '%s': %v", f.Name, err)
		}
	}

	stmts, err := compiler.CompileCreateTable(col)
	if err != nil {
		t.Fatalf("CompileCreateTable() error = %v", err)
	}

	allSQL := strings.Join(stmts, "\n")

	// Verify ON DELETE SET NULL uses valid SQL syntax with space
	if !strings.Contains(allSQL, "ON DELETE SET NULL") {
		t.Fatalf("expected 'ON DELETE SET NULL' with space in SQL:\n%s", allSQL)
	}
	// Verify O2M does not create physical column
	if strings.Contains(allSQL, "child_items") {
		t.Fatalf("expected O2M relation to not generate physical column:\n%s", allSQL)
	}
	// Verify boolean DEFAULT true is clean without duplicates
	if !strings.Contains(allSQL, "is_verified BOOLEAN DEFAULT true NULL") {
		t.Fatalf("expected clean boolean default without duplication in SQL:\n%s", allSQL)
	}
	// Verify encrypted field maps to TEXT
	if !strings.Contains(allSQL, "api_secret TEXT NULL") {
		t.Fatalf("expected encrypted field to map to TEXT in SQL:\n%s", allSQL)
	}
	// Verify partial unique index for unique field
	if !strings.Contains(allSQL, "CREATE UNIQUE INDEX uq_accounts_email ON usr_accounts(tenant_id, email) WHERE deleted_at IS NULL;") {
		t.Fatalf("expected unique index for email in SQL:\n%s", allSQL)
	}
}

func TestDDLCompiler_SafeIdentifierTruncation(t *testing.T) {
	t.Parallel()

	id := database.SafeIdentifier("fk", "very_long_collection_name_exceeding_standard_postgres_limits_table", "very_long_target_reference_column_identifier")
	if len(id) > 63 {
		t.Fatalf("SafeIdentifier length = %d exceeds PostgreSQL 63 byte limit: %s", len(id), id)
	}
}

func TestDDLCompiler_CompileAddColumn(t *testing.T) {
	t.Parallel()

	compiler := database.NewDDLCompiler()
	col, err := core.NewCollection(1, "customers", "Customers")
	if err != nil {
		t.Fatalf("unexpected error creating collection: %v", err)
	}

	field := &core.Field{
		Name:         "phone",
		Type:         core.FieldTypeText,
		IsRequired:   false,
		IsSearchable: true,
	}

	stmts, err := compiler.CompileAddColumn(col, field)
	if err != nil {
		t.Fatalf("CompileAddColumn() error = %v", err)
	}

	allSQL := strings.Join(stmts, "\n")
	if !strings.Contains(allSQL, "ALTER TABLE usr_customers ADD COLUMN phone TEXT NULL;") {
		t.Fatalf("expected ADD COLUMN statement in SQL:\n%s", allSQL)
	}
	if !strings.Contains(allSQL, "CREATE INDEX idx_customers_phone ON usr_customers(tenant_id, phone) WHERE deleted_at IS NULL;") {
		t.Fatalf("expected searchable index statement in SQL:\n%s", allSQL)
	}
}

func TestDDLCompiler_CompileTombstoneColumn(t *testing.T) {
	t.Parallel()

	compiler := database.NewDDLCompiler()
	col, err := core.NewCollection(1, "customers", "Customers")
	if err != nil {
		t.Fatalf("unexpected error creating collection: %v", err)
	}

	stmts, err := compiler.CompileTombstoneColumn(col, "legacy_field", 1730700000)
	if err != nil {
		t.Fatalf("CompileTombstoneColumn() error = %v", err)
	}

	allSQL := strings.Join(stmts, "\n")
	expected := "ALTER TABLE usr_customers RENAME COLUMN legacy_field TO tombstone_legacy_field_1730700000;"
	if !strings.Contains(allSQL, expected) {
		t.Fatalf("expected rename statement %q in SQL:\n%s", expected, allSQL)
	}
}
