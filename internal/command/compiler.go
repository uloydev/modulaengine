package command

import (
	"fmt"
	"strings"

	"github.com/uloydev/modulaengine/internal/core"
	"github.com/uloydev/modulaengine/internal/query"
)

type Compiler struct{}

func NewCompiler() *Compiler {
	return &Compiler{}
}

func (c *Compiler) isFieldPermitted(fieldName string, secCtx *core.SecurityContext) bool {
	if secCtx == nil || secCtx.IsSuperAdmin {
		return true
	}
	for _, f := range secCtx.AllowedFields {
		if f == "*" || f == fieldName {
			return true
		}
	}
	return false
}

func (c *Compiler) CompileInsert(col *core.Collection, data map[string]any, secCtx *core.SecurityContext) (*CompiledMutation, error) {
	if col == nil {
		return nil, fmt.Errorf("%w: collection cannot be nil", core.ErrValidation)
	}
	if secCtx == nil {
		return nil, fmt.Errorf("%w: security context required", core.ErrValidation)
	}

	cols := make([]string, 0, len(data)+4)
	placeholders := make([]string, 0, len(data)+4)
	args := make([]any, 0, len(data)+4)
	paramIdx := 1

	cols = append(cols, "tenant_id", "created_by", "updated_by", "version")
	placeholders = append(placeholders, fmt.Sprintf("$%d", paramIdx), fmt.Sprintf("$%d", paramIdx+1), fmt.Sprintf("$%d", paramIdx+2), "1")
	args = append(args, secCtx.TenantID, secCtx.UserID, secCtx.UserID)
	paramIdx += 3

	for _, field := range col.FieldList {
		if field.IsRequired && field.DefaultValue == nil && field.Type != core.FieldTypeBoolean && field.Type != core.FieldTypeJSON {
			val, exists := data[field.Name]
			if !exists || val == nil {
				return nil, fmt.Errorf("%w: missing or null required field '%s'", core.ErrValidation, field.Name)
			}
			if str, ok := val.(string); ok && strings.TrimSpace(str) == "" && field.Type == core.FieldTypeText {
				return nil, fmt.Errorf("%w: required text field '%s' cannot be empty", core.ErrValidation, field.Name)
			}
		}
	}

	diff := make(AuditDiff)

	for k, val := range data {
		field, ok := col.Fields[k]
		if !ok {
			if core.IsReservedIdentifier(k) {
				continue
			}
			return nil, fmt.Errorf("%w: unknown field '%s'", query.ErrUnknownField, k)
		}
		if !c.isFieldPermitted(k, secCtx) {
			return nil, fmt.Errorf("%w: cannot write field '%s'", query.ErrFieldNotPermitted, k)
		}

		if field.Type == core.FieldTypeEnum {
			strVal, okStr := val.(string)
			if !okStr {
				return nil, fmt.Errorf("%w: enum field '%s' must be string", core.ErrValidation, k)
			}
			matched := false
			for _, opt := range field.Options {
				if opt == strVal {
					matched = true
					break
				}
			}
			if !matched {
				return nil, fmt.Errorf("%w: invalid option '%s' for enum field '%s'", core.ErrValidation, strVal, k)
			}
		}

		cols = append(cols, k)
		placeholders = append(placeholders, fmt.Sprintf("$%d", paramIdx))
		args = append(args, val)
		paramIdx++

		diff[k] = FieldDiff{
			Old: nil,
			New: val,
		}
	}

	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING id, public_id, version, created_at, updated_at;",
		col.TableName,
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)

	return &CompiledMutation{
		SQL:       sql,
		Args:      args,
		Action:    ActionInsert,
		Columns:   cols,
		FieldDiff: diff,
	}, nil
}

func (c *Compiler) CompileUpdate(col *core.Collection, publicID string, expectedVersion int, data map[string]any, secCtx *core.SecurityContext) (*CompiledMutation, error) {
	if col == nil {
		return nil, fmt.Errorf("%w: collection cannot be nil", core.ErrValidation)
	}
	if secCtx == nil {
		return nil, fmt.Errorf("%w: security context required", core.ErrValidation)
	}
	if publicID == "" {
		return nil, fmt.Errorf("%w: public id cannot be empty", core.ErrValidation)
	}
	if expectedVersion < 1 {
		return nil, fmt.Errorf("%w: expected version must be positive integer", core.ErrValidation)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: update data cannot be empty", core.ErrValidation)
	}

	setClauses := make([]string, 0, len(data)+2)
	args := make([]any, 0, len(data)+4)
	paramIdx := 1

	diff := make(AuditDiff)

	for k, val := range data {
		if core.IsReservedIdentifier(k) {
			return nil, fmt.Errorf("%w: cannot directly mutate system column '%s'", core.ErrValidation, k)
		}
		field, ok := col.Fields[k]
		if !ok {
			return nil, fmt.Errorf("%w: unknown field '%s'", query.ErrUnknownField, k)
		}
		if !c.isFieldPermitted(k, secCtx) {
			return nil, fmt.Errorf("%w: cannot write field '%s'", query.ErrFieldNotPermitted, k)
		}

		if field.Type == core.FieldTypeEnum {
			strVal, okStr := val.(string)
			if !okStr {
				return nil, fmt.Errorf("%w: enum field '%s' must be string", core.ErrValidation, k)
			}
			matched := false
			for _, opt := range field.Options {
				if opt == strVal {
					matched = true
					break
				}
			}
			if !matched {
				return nil, fmt.Errorf("%w: invalid option '%s' for enum field '%s'", core.ErrValidation, strVal, k)
			}
		}

		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", k, paramIdx))
		args = append(args, val)
		paramIdx++

		diff[k] = FieldDiff{
			Old: nil, // Old value will be resolved via transaction pre-read or CTE
			New: val,
		}
	}

	setClauses = append(setClauses,
		"version = version + 1",
		"updated_at = now()",
		fmt.Sprintf("updated_by = $%d", paramIdx),
	)
	args = append(args, secCtx.UserID)
	paramIdx++

	publicIDParam := paramIdx
	args = append(args, publicID)
	paramIdx++

	versionParam := paramIdx
	args = append(args, expectedVersion)
	paramIdx++

	tenantIDParam := paramIdx
	args = append(args, secCtx.TenantID)

	sql := fmt.Sprintf("UPDATE %s SET %s WHERE public_id = $%d AND version = $%d AND tenant_id = $%d AND deleted_at IS NULL RETURNING id, public_id, version, updated_at;",
		col.TableName,
		strings.Join(setClauses, ", "),
		publicIDParam,
		versionParam,
		tenantIDParam,
	)

	return &CompiledMutation{
		SQL:       sql,
		Args:      args,
		Action:    ActionUpdate,
		FieldDiff: diff,
	}, nil
}

func (c *Compiler) CompileSoftDelete(col *core.Collection, publicID string, expectedVersion int, secCtx *core.SecurityContext) (*CompiledMutation, error) {
	if col == nil {
		return nil, fmt.Errorf("%w: collection cannot be nil", core.ErrValidation)
	}
	if secCtx == nil {
		return nil, fmt.Errorf("%w: security context required", core.ErrValidation)
	}
	if publicID == "" {
		return nil, fmt.Errorf("%w: public id cannot be empty", core.ErrValidation)
	}
	if expectedVersion < 1 {
		return nil, fmt.Errorf("%w: expected version must be positive integer", core.ErrValidation)
	}

	args := make([]any, 0, 4)
	paramIdx := 1

	userIDParam := paramIdx
	args = append(args, secCtx.UserID)
	paramIdx++

	publicIDParam := paramIdx
	args = append(args, publicID)
	paramIdx++

	versionParam := paramIdx
	args = append(args, expectedVersion)
	paramIdx++

	tenantIDParam := paramIdx
	args = append(args, secCtx.TenantID)

	sql := fmt.Sprintf("UPDATE %s SET deleted_at = now(), deleted_by = $%d, version = version + 1 WHERE public_id = $%d AND version = $%d AND tenant_id = $%d AND deleted_at IS NULL RETURNING id, public_id, version, deleted_at;",
		col.TableName,
		userIDParam,
		publicIDParam,
		versionParam,
		tenantIDParam,
	)

	return &CompiledMutation{
		SQL:    sql,
		Args:   args,
		Action: ActionDelete,
	}, nil
}
