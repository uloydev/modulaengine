package core_test

import (
	"errors"
	"testing"

	"github.com/uloydev/modulaengine/internal/core"
)

func TestValidateIdentifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid simple", input: "users", wantErr: false},
		{name: "valid with underscore", input: "order_items", wantErr: false},
		{name: "valid with numbers", input: "item_2026", wantErr: false},
		{name: "invalid starts with number", input: "1orders", wantErr: true},
		{name: "invalid uppercase", input: "Orders", wantErr: true},
		{name: "invalid special chars", input: "order-items", wantErr: true},
		{name: "invalid empty", input: "", wantErr: true},
		{name: "invalid space", input: "order items", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := core.ValidateIdentifier(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateIdentifier(%q) error = %v, wantErr = %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, core.ErrInvalidIdentifier) {
				t.Fatalf("expected ErrInvalidIdentifier, got %v", err)
			}
		})
	}
}

func TestCollection_AddField(t *testing.T) {
	t.Parallel()

	col, err := core.NewCollection(1, "products", "Products")
	if err != nil {
		t.Fatalf("unexpected error creating collection: %v", err)
	}

	tests := []struct {
		name    string
		field   *core.Field
		wantErr bool
		errIs   error
	}{
		{
			name: "valid text field",
			field: &core.Field{
				Name:       "title",
				Type:       core.FieldTypeText,
				IsRequired: true,
			},
			wantErr: false,
		},
		{
			name: "reserved identifier",
			field: &core.Field{
				Name: "version",
				Type: core.FieldTypeNumber,
			},
			wantErr: true,
			errIs:   core.ErrReservedName,
		},
		{
			name: "invalid enum without options",
			field: &core.Field{
				Name: "status",
				Type: core.FieldTypeEnum,
			},
			wantErr: true,
			errIs:   core.ErrValidation,
		},
		{
			name: "valid enum with options",
			field: &core.Field{
				Name:    "status",
				Type:    core.FieldTypeEnum,
				Options: []string{"DRAFT", "PUBLISHED"},
			},
			wantErr: false,
		},
		{
			name: "duplicate field",
			field: &core.Field{
				Name: "title",
				Type: core.FieldTypeText,
			},
			wantErr: true,
			errIs:   core.ErrFieldExists,
		},
		{
			name: "valid relation field",
			field: &core.Field{
				Name: "category_id",
				Type: core.FieldTypeRelation,
				Relation: &core.RelationDef{
					RelationType:     "M2O",
					TargetCollection: "categories",
					OnDelete:         core.OnDeleteSetNull,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid relation without definition",
			field: &core.Field{
				Name: "parent_id",
				Type: core.FieldTypeRelation,
			},
			wantErr: true,
			errIs:   core.ErrValidation,
		},
		{
			name: "invalid relation type",
			field: &core.Field{
				Name: "supplier_id",
				Type: core.FieldTypeRelation,
				Relation: &core.RelationDef{
					RelationType:     "INVALID",
					TargetCollection: "suppliers",
				},
			},
			wantErr: true,
			errIs:   core.ErrValidation,
		},
		{
			name: "invalid encrypted searchable",
			field: &core.Field{
				Name:         "secret_code",
				Type:         core.FieldTypeText,
				IsEncrypted:  true,
				IsSearchable: true,
			},
			wantErr: true,
			errIs:   core.ErrValidation,
		},
		{
			name: "invalid numeric scale exceeding precision",
			field: func() *core.Field {
				p, s := 5, 8
				return &core.Field{
					Name:      "ratio",
					Type:      core.FieldTypeNumber,
					Precision: &p,
					Scale:     &s,
				}
			}(),
			wantErr: true,
			errIs:   core.ErrValidation,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := col.AddField(tt.field)
			if (err != nil) != tt.wantErr {
				t.Fatalf("AddField() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.errIs != nil && !errors.Is(err, tt.errIs) {
				t.Fatalf("expected error %v, got %v", tt.errIs, err)
			}
		})
	}
}
