package database

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/uloydev/modulaengine/internal/core"
)

type DDLCompiler struct{}

func NewDDLCompiler() *DDLCompiler {
	return &DDLCompiler{}
}

func SafeIdentifier(parts ...string) string {
	raw := strings.Join(parts, "_")
	if len(raw) <= 63 {
		return raw
	}
	hash := sha256.Sum256([]byte(raw))
	hashSuffix := hex.EncodeToString(hash[:4])
	trimmed := raw
	if len(trimmed) > 54 {
		trimmed = trimmed[:54]
	}
	return fmt.Sprintf("%s_%s", trimmed, hashSuffix)
}

func (c *DDLCompiler) CompileCreateTable(col *core.Collection) ([]string, error) {
	if err := core.ValidateIdentifier(col.Name); err != nil {
		return nil, fmt.Errorf("compile create table: %w", err)
	}

	tableName := "usr_" + col.Name
	var lines []string

	lines = append(lines,
		`id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY`,
		`public_id UUID NOT NULL DEFAULT gen_random_uuid()`,
		`tenant_id BIGINT NOT NULL REFERENCES sys_tenants(id) ON DELETE CASCADE`,
		`created_at TIMESTAMPTZ NOT NULL DEFAULT now()`,
		`created_by BIGINT NOT NULL REFERENCES sys_users(id)`,
		`updated_at TIMESTAMPTZ NOT NULL DEFAULT now()`,
		`updated_by BIGINT NOT NULL REFERENCES sys_users(id)`,
		`deleted_at TIMESTAMPTZ NULL`,
		`deleted_by BIGINT NULL REFERENCES sys_users(id)`,
		`version INT NOT NULL DEFAULT 1`,
	)

	var constraints []string
	uqTenantID := SafeIdentifier("uq", col.Name, "tenant_id")
	uqTenantPublicID := SafeIdentifier("uq", col.Name, "tenant_public_id")
	constraints = append(constraints,
		fmt.Sprintf(`CONSTRAINT %s UNIQUE (tenant_id, id)`, uqTenantID),
		fmt.Sprintf(`CONSTRAINT %s UNIQUE (tenant_id, public_id)`, uqTenantPublicID),
	)

	var indexes []string
	idxTenantActive := SafeIdentifier("idx", col.Name, "tenant_active")
	idxPublicLookup := SafeIdentifier("idx", col.Name, "public_lookup")
	indexes = append(indexes,
		fmt.Sprintf(`CREATE INDEX %s ON %s(tenant_id) WHERE deleted_at IS NULL;`, idxTenantActive, tableName),
		fmt.Sprintf(`CREATE INDEX %s ON %s(tenant_id, public_id) WHERE deleted_at IS NULL;`, idxPublicLookup, tableName),
	)

	for _, f := range col.FieldList {
		colDef, colConstraints, isPhysical, err := c.compileColumnDefinition(col, f)
		if err != nil {
			return nil, fmt.Errorf("compile field '%s': %w", f.Name, err)
		}
		if !isPhysical {
			continue
		}

		lines = append(lines, colDef)
		if len(colConstraints) > 0 {
			constraints = append(constraints, colConstraints...)
		}

		if f.IsUnique {
			uqIdxName := SafeIdentifier("uq", col.Name, f.Name)
			indexes = append(indexes,
				fmt.Sprintf(`CREATE UNIQUE INDEX %s ON %s(tenant_id, %s) WHERE deleted_at IS NULL;`, uqIdxName, tableName, f.Name),
			)
		}

		if f.IsSearchable {
			idxName := SafeIdentifier("idx", col.Name, f.Name)
			indexes = append(indexes,
				fmt.Sprintf(`CREATE INDEX %s ON %s(tenant_id, %s) WHERE deleted_at IS NULL;`, idxName, tableName, f.Name),
			)
		} else if f.Type == core.FieldTypeRelation && f.Relation != nil && f.Relation.RelationType == core.RelationTypeM2O {
			idxName := SafeIdentifier("idx", col.Name, f.Name)
			indexes = append(indexes,
				fmt.Sprintf(`CREATE INDEX %s ON %s(tenant_id, %s) WHERE deleted_at IS NULL;`, idxName, tableName, f.Name),
			)
		}
	}

	allTableDefs := append(lines, constraints...)
	createTableSQL := fmt.Sprintf("CREATE TABLE %s (\n    %s\n);", tableName, strings.Join(allTableDefs, ",\n    "))

	statements := make([]string, 0, len(indexes)+3)
	statements = append(statements,
		"SET LOCAL lock_timeout = '2s';",
		"SET LOCAL statement_timeout = '5s';",
		createTableSQL,
	)
	statements = append(statements, indexes...)

	return statements, nil
}

func (c *DDLCompiler) CompileAddColumn(col *core.Collection, f *core.Field) ([]string, error) {
	if err := f.Validate(); err != nil {
		return nil, fmt.Errorf("validate field for add column: %w", err)
	}

	if f.IsRequired && f.DefaultValue == nil && f.Type != core.FieldTypeBoolean && f.Type != core.FieldTypeJSON {
		return nil, fmt.Errorf("%w: cannot add NOT NULL column '%s' without DEFAULT value to existing table", core.ErrValidation, f.Name)
	}

	colDef, colConstraints, isPhysical, err := c.compileColumnDefinition(col, f)
	if err != nil {
		return nil, fmt.Errorf("compile column definition: %w", err)
	}
	if !isPhysical {
		return nil, nil
	}

	tableName := "usr_" + col.Name
	statements := []string{
		"SET LOCAL lock_timeout = '2s';",
		"SET LOCAL statement_timeout = '5s';",
		fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;", tableName, colDef),
	}

	for _, constraint := range colConstraints {
		statements = append(statements, fmt.Sprintf("ALTER TABLE %s ADD %s;", tableName, constraint))
	}

	if f.IsUnique {
		uqIdxName := SafeIdentifier("uq", col.Name, f.Name)
		statements = append(statements, fmt.Sprintf("CREATE UNIQUE INDEX %s ON %s(tenant_id, %s) WHERE deleted_at IS NULL;", uqIdxName, tableName, f.Name))
	}

	if f.IsSearchable || (f.Type == core.FieldTypeRelation && f.Relation != nil && f.Relation.RelationType == core.RelationTypeM2O) {
		idxName := SafeIdentifier("idx", col.Name, f.Name)
		statements = append(statements, fmt.Sprintf("CREATE INDEX %s ON %s(tenant_id, %s) WHERE deleted_at IS NULL;", idxName, tableName, f.Name))
	}

	return statements, nil
}

func (c *DDLCompiler) CompileTombstoneColumn(col *core.Collection, fieldName string, timestamp int64) ([]string, error) {
	if err := core.ValidateIdentifier(fieldName); err != nil {
		return nil, fmt.Errorf("validate field for tombstone: %w", err)
	}

	tableName := "usr_" + col.Name
	tombstoneName := fmt.Sprintf("tombstone_%s_%d", fieldName, timestamp)

	statements := []string{
		"SET LOCAL lock_timeout = '2s';",
		"SET LOCAL statement_timeout = '5s';",
		fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s;", tableName, fieldName, tombstoneName),
	}

	return statements, nil
}

func (c *DDLCompiler) compileColumnDefinition(col *core.Collection, f *core.Field) (string, []string, bool, error) {
	if f.Type == core.FieldTypeRelation && f.Relation != nil && f.Relation.RelationType == core.RelationTypeO2M {
		return "", nil, false, nil
	}

	var constraints []string
	var typeClause string

	if f.IsEncrypted {
		typeClause = "TEXT"
	} else {
		switch f.Type {
		case core.FieldTypeText:
			typeClause = "TEXT"
		case core.FieldTypeNumber:
			if f.Precision != nil && f.Scale != nil {
				typeClause = fmt.Sprintf("NUMERIC(%d,%d)", *f.Precision, *f.Scale)
			} else {
				typeClause = "NUMERIC"
			}
		case core.FieldTypeBoolean:
			typeClause = "BOOLEAN"
		case core.FieldTypeDateTime:
			typeClause = "TIMESTAMPTZ"
		case core.FieldTypeJSON:
			typeClause = "JSONB"
		case core.FieldTypeEnum:
			typeClause = "VARCHAR(64)"
			escapedOptions := make([]string, len(f.Options))
			for i, opt := range f.Options {
				cleanOpt := strings.ReplaceAll(opt, "'", "''")
				escapedOptions[i] = fmt.Sprintf("'%s'", cleanOpt)
			}
			chkName := SafeIdentifier("chk", col.Name, f.Name)
			checkConstraint := fmt.Sprintf("CONSTRAINT %s CHECK (%s IN (%s))",
				chkName, f.Name, strings.Join(escapedOptions, ", "))
			constraints = append(constraints, checkConstraint)
		case core.FieldTypeRelation:
			typeClause = "BIGINT"
			if f.Relation != nil {
				targetTable := "usr_" + f.Relation.TargetCollection
				fkName := SafeIdentifier("fk", col.Name, f.Name)
				fkConstraint := fmt.Sprintf("CONSTRAINT %s FOREIGN KEY (tenant_id, %s) REFERENCES %s(tenant_id, id) ON DELETE %s",
					fkName, f.Name, targetTable, f.Relation.OnDelete.SQL())
				constraints = append(constraints, fkConstraint)
			}
		default:
			return "", nil, false, fmt.Errorf("unsupported field type: %s", f.Type)
		}
	}

	defaultClause, err := c.compileDefaultValue(f)
	if err != nil {
		return "", nil, false, err
	}
	if defaultClause != "" {
		defaultClause = " " + defaultClause
	}

	var nullClause string
	if f.IsRequired {
		nullClause = " NOT NULL"
	} else {
		nullClause = " NULL"
	}

	columnDef := fmt.Sprintf("%s %s%s%s", f.Name, typeClause, defaultClause, nullClause)
	return columnDef, constraints, true, nil
}

func (c *DDLCompiler) compileDefaultValue(f *core.Field) (string, error) {
	if f.DefaultValue == nil {
		switch f.Type {
		case core.FieldTypeBoolean:
			return "DEFAULT false", nil
		case core.FieldTypeJSON:
			return "DEFAULT '{}'::jsonb", nil
		default:
			return "", nil
		}
	}

	val := *f.DefaultValue
	switch f.Type {
	case core.FieldTypeBoolean:
		switch strings.ToLower(val) {
		case "true", "t", "1":
			return "DEFAULT true", nil
		case "false", "f", "0":
			return "DEFAULT false", nil
		default:
			return "", fmt.Errorf("invalid boolean default: %s", val)
		}
	case core.FieldTypeNumber:
		clean := strings.TrimSpace(val)
		return fmt.Sprintf("DEFAULT %s", clean), nil
	case core.FieldTypeJSON:
		clean := strings.ReplaceAll(val, "'", "''")
		return fmt.Sprintf("DEFAULT '%s'::jsonb", clean), nil
	case core.FieldTypeDateTime:
		if strings.EqualFold(val, "now()") || strings.EqualFold(val, "current_timestamp") {
			return "DEFAULT now()", nil
		}
		clean := strings.ReplaceAll(val, "'", "''")
		return fmt.Sprintf("DEFAULT '%s'", clean), nil
	case core.FieldTypeText, core.FieldTypeEnum:
		clean := strings.ReplaceAll(val, "'", "''")
		return fmt.Sprintf("DEFAULT '%s'", clean), nil
	case core.FieldTypeRelation:
		clean := strings.TrimSpace(val)
		return fmt.Sprintf("DEFAULT %s", clean), nil
	default:
		clean := strings.ReplaceAll(val, "'", "''")
		return fmt.Sprintf("DEFAULT '%s'", clean), nil
	}
}
