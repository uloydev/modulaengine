package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/uloydev/modulaengine/internal/core"
)

type BatchQueryFunc func(ctx context.Context, col *core.Collection, ids []any, secCtx *core.SecurityContext) ([]map[string]any, error)

type RelationResolver struct {
	registry *Registry
	batchFn  BatchQueryFunc
}

func NewRelationResolver(registry *Registry, batchFn BatchQueryFunc) *RelationResolver {
	return &RelationResolver{
		registry: registry,
		batchFn:  batchFn,
	}
}

func (r *RelationResolver) Resolve(
	ctx context.Context,
	parentRecords []map[string]any,
	expansions []string,
	parentCol *core.Collection,
	secCtx *core.SecurityContext,
) error {
	return ResolveRelations(ctx, parentRecords, expansions, parentCol, secCtx, r.registry, r.batchFn)
}

func (r *RelationResolver) ResolveWithFunc(
	ctx context.Context,
	parentRecords []map[string]any,
	expansions []string,
	parentCol *core.Collection,
	secCtx *core.SecurityContext,
	batchFn BatchQueryFunc,
) error {
	return ResolveRelations(ctx, parentRecords, expansions, parentCol, secCtx, r.registry, batchFn)
}

func ResolveRelations(
	ctx context.Context,
	parentRecords []map[string]any,
	expansions []string,
	parentCol *core.Collection,
	secCtx *core.SecurityContext,
	registry *Registry,
	batchFn BatchQueryFunc,
) error {
	if len(parentRecords) == 0 || len(expansions) == 0 || parentCol == nil || batchFn == nil {
		return nil
	}

	for _, rawExp := range expansions {
		exp := strings.TrimSpace(rawExp)
		if exp == "" {
			continue
		}

		field, injectKey := findRelationField(parentCol, exp)
		if field == nil || field.Relation == nil || field.Relation.RelationType != core.RelationTypeM2O {
			continue
		}

		targetColName := field.Relation.TargetCollection
		var targetCol *core.Collection
		if registry != nil && secCtx != nil {
			targetCol, _ = registry.GetCollection(secCtx.TenantID, targetColName)
		}
		if targetCol == nil {
			targetCol = &core.Collection{
				TenantID:  secCtx.TenantID,
				Name:      targetColName,
				TableName: "usr_" + targetColName,
			}
		}

		seenIDs := make(map[any]struct{}, len(parentRecords))
		uniqueIDs := make([]any, 0, len(parentRecords))

		for _, record := range parentRecords {
			fkVal, exists := record[field.Name]
			if !exists || fkVal == nil {
				if strings.HasSuffix(field.Name, "_id") {
					shortName := strings.TrimSuffix(field.Name, "_id")
					if v2, ok2 := record[shortName]; ok2 && v2 != nil {
						if _, isMap := v2.(map[string]any); !isMap {
							fkVal = v2
							exists = true
						}
					}
				}
			}
			if !exists || fkVal == nil {
				continue
			}

			normKey := normalizeIDKey(fkVal)
			if _, seen := seenIDs[normKey]; !seen {
				seenIDs[normKey] = struct{}{}
				uniqueIDs = append(uniqueIDs, fkVal)
			}
		}

		if len(uniqueIDs) == 0 {
			for _, record := range parentRecords {
				record[injectKey] = nil
				if exp != injectKey {
					record[exp] = nil
				}
			}
			continue
		}

		targetRecords, err := batchFn(ctx, targetCol, uniqueIDs, secCtx)
		if err != nil {
			return fmt.Errorf("batch hydrate relation %s (%s): %w", exp, targetColName, err)
		}

		lookup := make(map[any]map[string]any, len(targetRecords))
		for _, tr := range targetRecords {
			if tr == nil {
				continue
			}
			if internalID, ok := tr["_internal_id"]; ok && internalID != nil {
				lookup[normalizeIDKey(internalID)] = tr
			}
			if idVal, ok := tr["id"]; ok && idVal != nil {
				lookup[normalizeIDKey(idVal)] = tr
			}
			if pubVal, ok := tr["public_id"]; ok && pubVal != nil {
				lookup[normalizeIDKey(pubVal)] = tr
			}
		}

		for _, record := range parentRecords {
			fkVal := record[field.Name]
			if fkVal == nil && strings.HasSuffix(field.Name, "_id") {
				fkVal = record[strings.TrimSuffix(field.Name, "_id")]
			}

			if fkVal != nil {
				if matched, found := lookup[normalizeIDKey(fkVal)]; found {
					record[injectKey] = matched
					if exp != injectKey {
						record[exp] = matched
					}
				} else {
					record[injectKey] = nil
					if exp != injectKey {
						record[exp] = nil
					}
				}
			} else {
				record[injectKey] = nil
				if exp != injectKey {
					record[exp] = nil
				}
			}
		}

		for _, tr := range targetRecords {
			delete(tr, "_internal_id")
		}
	}

	return nil
}

func findRelationField(col *core.Collection, expansion string) (*core.Field, string) {
	if col == nil {
		return nil, ""
	}

	if f, ok := col.Fields[expansion]; ok && f.Type == core.FieldTypeRelation && f.Relation != nil {
		return f, expansion
	}

	if f, ok := col.Fields[expansion+"_id"]; ok && f.Type == core.FieldTypeRelation && f.Relation != nil {
		return f, expansion
	}

	for _, f := range col.FieldList {
		if f.Type != core.FieldTypeRelation || f.Relation == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSuffix(f.Name, "_id"), expansion) {
			return f, expansion
		}
		if strings.EqualFold(f.Relation.TargetCollection, expansion) {
			return f, expansion
		}
		if strings.EqualFold(strings.TrimSuffix(f.Relation.TargetCollection, "s"), expansion) {
			return f, expansion
		}
	}

	return nil, ""
}

func normalizeIDKey(val any) any {
	switch v := val.(type) {
	case int:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	case uint:
		return int64(v)
	case uint32:
		return int64(v)
	case uint64:
		return int64(v)
	case float64:
		return int64(v)
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (c *Compiler) CompileBatchSelect(col *core.Collection, ids []any, secCtx *core.SecurityContext) (*CompiledQuery, error) {
	if col == nil {
		return nil, fmt.Errorf("%w: collection cannot be nil", core.ErrValidation)
	}
	if secCtx == nil {
		return nil, fmt.Errorf("%w: security context required", core.ErrValidation)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("%w: ids cannot be empty", core.ErrValidation)
	}

	cols, err := c.resolveBatchProjections(col, secCtx)
	if err != nil {
		return nil, err
	}

	isInt := true
	for _, id := range ids {
		switch id.(type) {
		case int, int32, int64, uint, uint32, uint64, float64:
		default:
			isInt = false
		}
		if !isInt {
			break
		}
	}

	var (
		whereCol string
		arrayArg any
	)

	if isInt {
		whereCol = "id"
		int64IDs := make([]int64, len(ids))
		for i, id := range ids {
			switch v := id.(type) {
			case int:
				int64IDs[i] = int64(v)
			case int32:
				int64IDs[i] = int64(v)
			case int64:
				int64IDs[i] = v
			case uint:
				int64IDs[i] = int64(v)
			case uint32:
				int64IDs[i] = int64(v)
			case uint64:
				int64IDs[i] = int64(v)
			case float64:
				int64IDs[i] = int64(v)
			}
		}
		arrayArg = int64IDs
	} else {
		whereCol = "public_id"
		strIDs := make([]string, len(ids))
		for i, id := range ids {
			strIDs[i] = fmt.Sprintf("%v", id)
		}
		arrayArg = strIDs
	}

	sql := fmt.Sprintf("SELECT %s FROM %s WHERE %s = ANY($1) AND tenant_id = $2 AND deleted_at IS NULL;",
		strings.Join(cols, ", "),
		col.TableName,
		whereCol,
	)

	args := []any{arrayArg, secCtx.TenantID}

	return &CompiledQuery{
		SQL:     sql,
		Args:    args,
		Columns: cols,
	}, nil
}

func (c *Compiler) resolveBatchProjections(col *core.Collection, secCtx *core.SecurityContext) ([]string, error) {
	proj := make([]string, 0, len(col.FieldList)+5)
	proj = append(proj, "id AS _internal_id", "public_id AS id")

	for _, f := range col.FieldList {
		if c.isFieldPermitted(f.Name, secCtx) {
			proj = append(proj, f.Name)
		}
	}
	proj = append(proj, "version", "created_at", "updated_at")
	return proj, nil
}
