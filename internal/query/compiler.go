package query

import (
	"fmt"
	"strings"

	"github.com/uloydev/modulaengine/internal/core"
)

type CompiledQuery struct {
	SQL           string
	Args          []any
	Columns       []string
	FieldMappings map[string]string
}

func (q *CompiledQuery) Clone() *CompiledQuery {
	if q == nil {
		return nil
	}
	var argsCopy []any
	if q.Args != nil {
		argsCopy = make([]any, len(q.Args))
		copy(argsCopy, q.Args)
	}
	var colsCopy []string
	if q.Columns != nil {
		colsCopy = make([]string, len(q.Columns))
		copy(colsCopy, q.Columns)
	}
	var mappingsCopy map[string]string
	if q.FieldMappings != nil {
		mappingsCopy = make(map[string]string, len(q.FieldMappings))
		for k, v := range q.FieldMappings {
			mappingsCopy[k] = v
		}
	}
	return &CompiledQuery{
		SQL:           q.SQL,
		Args:          argsCopy,
		Columns:       colsCopy,
		FieldMappings: mappingsCopy,
	}
}

type Compiler struct {
	registry *Registry
}

func NewCompiler(registry *Registry) *Compiler {
	return &Compiler{registry: registry}
}

var standardFields = map[string]struct{}{
	"id":         {},
	"public_id":  {},
	"tenant_id":  {},
	"created_at": {},
	"created_by": {},
	"updated_at": {},
	"updated_by": {},
	"deleted_at": {},
	"deleted_by": {},
	"version":    {},
}

func isFieldKnown(col *core.Collection, fieldName string) bool {
	if _, ok := standardFields[fieldName]; ok {
		return true
	}
	_, ok := col.Fields[fieldName]
	return ok
}

func (c *Compiler) isFieldPermitted(fieldName string, secCtx *core.SecurityContext) bool {
	if secCtx == nil || secCtx.IsSuperAdmin {
		return true
	}
	if _, ok := standardFields[fieldName]; ok {
		return true
	}
	for _, f := range secCtx.AllowedFields {
		if f == "*" || f == fieldName {
			return true
		}
	}
	return false
}

func (c *Compiler) CompileSelect(col *core.Collection, params *QueryParams, secCtx *core.SecurityContext) (*CompiledQuery, error) {
	if col == nil {
		return nil, fmt.Errorf("%w: collection cannot be nil", core.ErrValidation)
	}
	if secCtx == nil {
		return nil, fmt.Errorf("%w: security context required", core.ErrValidation)
	}

	args := make([]any, 0, 16)
	paramIdx := 1

	tenantIDParam := paramIdx
	args = append(args, secCtx.TenantID)
	paramIdx++

	cols, err := c.resolveProjections(col, params, secCtx)
	if err != nil {
		return nil, err
	}

	wherePredicates := make([]string, 0, 4)
	wherePredicates = append(wherePredicates, "deleted_at IS NULL")
	wherePredicates = append(wherePredicates, fmt.Sprintf("tenant_id = $%d", tenantIDParam))

	if params != nil && params.Filter != nil {
		userFilterSQL, nextParam, err := c.compileNode(col, params.Filter, secCtx, paramIdx, &args, false)
		if err != nil {
			return nil, fmt.Errorf("compile user filter: %w", err)
		}
		if userFilterSQL != "" {
			wherePredicates = append(wherePredicates, fmt.Sprintf("(%s)", userFilterSQL))
			paramIdx = nextParam
		}
	}

	if secCtx.RLSFilter != "" {
		rlsNode, err := ParseRLSFilter(secCtx.RLSFilter, secCtx)
		if err != nil {
			return nil, fmt.Errorf("parse rls filter: %w", err)
		}
		if rlsNode != nil {
			rlsSQL, nextParam, err := c.compileNode(col, rlsNode, secCtx, paramIdx, &args, true)
			if err != nil {
				return nil, fmt.Errorf("compile rls filter: %w", err)
			}
			if rlsSQL != "" {
				wherePredicates = append(wherePredicates, fmt.Sprintf("(%s)", rlsSQL))
				paramIdx = nextParam
			}
		}
	}

	sortSQL, err := c.compileSort(col, params, secCtx)
	if err != nil {
		return nil, err
	}

	limit := 25
	offset := 0
	if params != nil {
		if params.Limit > 0 {
			limit = params.Limit
		}
		if params.Offset >= 0 {
			offset = params.Offset
		}
	}

	limitParam := paramIdx
	args = append(args, limit)
	paramIdx++

	offsetParam := paramIdx
	args = append(args, offset)

	sql := fmt.Sprintf("SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d;",
		strings.Join(cols, ", "),
		col.TableName,
		strings.Join(wherePredicates, " AND "),
		sortSQL,
		limitParam,
		offsetParam,
	)

	return &CompiledQuery{
		SQL:     sql,
		Args:    args,
		Columns: cols,
	}, nil
}

func (c *Compiler) resolveProjections(col *core.Collection, params *QueryParams, secCtx *core.SecurityContext) ([]string, error) {
	proj := make([]string, 0, len(col.FieldList)+4)
	proj = append(proj, "public_id AS id")

	if params != nil && len(params.Fields) > 0 {
		for _, f := range params.Fields {
			if f == "id" {
				continue
			}
			if !isFieldKnown(col, f) {
				return nil, fmt.Errorf("%w: requested field '%s'", ErrUnknownField, f)
			}
			if !c.isFieldPermitted(f, secCtx) {
				return nil, fmt.Errorf("%w: field '%s'", ErrFieldNotPermitted, f)
			}
			proj = append(proj, f)
		}
	} else {
		for _, f := range col.FieldList {
			if c.isFieldPermitted(f.Name, secCtx) {
				proj = append(proj, f.Name)
			}
		}
		proj = append(proj, "version", "created_at", "updated_at")
	}

	return proj, nil
}

func (c *Compiler) compileSort(col *core.Collection, params *QueryParams, secCtx *core.SecurityContext) (string, error) {
	if params == nil || len(params.Sort) == 0 {
		return "id ASC", nil
	}

	clauses := make([]string, 0, len(params.Sort))
	for _, sf := range params.Sort {
		if !isFieldKnown(col, sf.Field) {
			return "", fmt.Errorf("%w: sort field '%s'", ErrUnknownField, sf.Field)
		}
		if !c.isFieldPermitted(sf.Field, secCtx) {
			return "", fmt.Errorf("%w: sort field '%s'", ErrFieldNotPermitted, sf.Field)
		}
		order := "ASC"
		if sf.Order == SortDesc {
			order = "DESC"
		}
		clauses = append(clauses, fmt.Sprintf("%s %s", sf.Field, order))
	}

	return strings.Join(clauses, ", "), nil
}

func (c *Compiler) compileNode(col *core.Collection, n Node, secCtx *core.SecurityContext, startParam int, args *[]any, isRLS bool) (string, int, error) {
	paramIdx := startParam

	switch node := n.(type) {
	case *ComparisonNode:
		if !isFieldKnown(col, node.Field) {
			return "", paramIdx, fmt.Errorf("%w: '%s'", ErrUnknownField, node.Field)
		}
		if !isRLS && !c.isFieldPermitted(node.Field, secCtx) {
			return "", paramIdx, fmt.Errorf("%w: '%s'", ErrFieldNotPermitted, node.Field)
		}

		if node.Op == OpIn || node.Op == OpNin {
			sliceVal, ok := node.Value.([]any)
			if !ok {
				strSlice, okStr := node.Value.([]string)
				if !okStr {
					return "", paramIdx, fmt.Errorf("%w: invalid in-array type", ErrInvalidFilterNode)
				}
				sliceVal = make([]any, len(strSlice))
				for i, s := range strSlice {
					sliceVal[i] = s
				}
			}

			placeholders := make([]string, len(sliceVal))
			for i, v := range sliceVal {
				placeholders[i] = fmt.Sprintf("$%d", paramIdx)
				*args = append(*args, v)
				paramIdx++
			}
			opStr := "IN"
			if node.Op == OpNin {
				opStr = "NOT IN"
			}
			return fmt.Sprintf("%s %s (%s)", node.Field, opStr, strings.Join(placeholders, ", ")), paramIdx, nil
		}

		sqlOp, err := node.Op.SQL()
		if err != nil {
			return "", paramIdx, err
		}

		placeholder := fmt.Sprintf("$%d", paramIdx)
		*args = append(*args, node.Value)
		paramIdx++

		return fmt.Sprintf("%s %s %s", node.Field, sqlOp, placeholder), paramIdx, nil

	case *NullNode:
		if !isFieldKnown(col, node.Field) {
			return "", paramIdx, fmt.Errorf("%w: '%s'", ErrUnknownField, node.Field)
		}
		if !isRLS && !c.isFieldPermitted(node.Field, secCtx) {
			return "", paramIdx, fmt.Errorf("%w: '%s'", ErrFieldNotPermitted, node.Field)
		}
		if node.IsNull {
			return fmt.Sprintf("%s IS NULL", node.Field), paramIdx, nil
		}
		return fmt.Sprintf("%s IS NOT NULL", node.Field), paramIdx, nil

	case *LogicalNode:
		parts := make([]string, 0, len(node.Children))
		for _, child := range node.Children {
			childSQL, nextParam, err := c.compileNode(col, child, secCtx, paramIdx, args, isRLS)
			if err != nil {
				return "", paramIdx, err
			}
			if childSQL != "" {
				parts = append(parts, childSQL)
				paramIdx = nextParam
			}
		}
		if len(parts) == 0 {
			return "", paramIdx, nil
		}
		if len(parts) == 1 {
			return parts[0], paramIdx, nil
		}
		joiner := " AND "
		if node.Op == LogicalOr {
			joiner = " OR "
		}
		return fmt.Sprintf("(%s)", strings.Join(parts, joiner)), paramIdx, nil

	default:
		return "", paramIdx, fmt.Errorf("%w: unknown node type", ErrInvalidFilterNode)
	}
}
