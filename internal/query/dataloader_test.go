package query_test

import (
	"context"
	"strings"
	"testing"

	"github.com/uloydev/modulaengine/internal/core"
	"github.com/uloydev/modulaengine/internal/query"
)

func TestRelationResolver_M2O_Batching(t *testing.T) {
	t.Parallel()

	registry := query.NewRegistry()

	// Orders collection with customer_id relation
	ordersCol, _ := core.NewCollection(1, "orders", "Orders")
	_ = ordersCol.AddField(&core.Field{
		Name: "order_number",
		Type: core.FieldTypeText,
	})
	_ = ordersCol.AddField(&core.Field{
		Name: "customer_id",
		Type: core.FieldTypeRelation,
		Relation: &core.RelationDef{
			RelationType:     core.RelationTypeM2O,
			TargetCollection: "customers",
		},
	})
	_ = registry.RegisterCollection(ordersCol)

	// Customers collection
	customersCol, _ := core.NewCollection(1, "customers", "Customers")
	_ = customersCol.AddField(&core.Field{
		Name: "name",
		Type: core.FieldTypeText,
	})
	_ = registry.RegisterCollection(customersCol)

	secCtx := &core.SecurityContext{
		TenantID:      1,
		UserID:        42,
		AllowedFields: []string{"*"},
	}

	parentRecords := []map[string]any{
		{"id": "ord-1", "order_number": "ORD-001", "customer_id": int64(10)},
		{"id": "ord-2", "order_number": "ORD-002", "customer_id": int64(20)},
		{"id": "ord-3", "order_number": "ORD-003", "customer_id": int64(10)}, // duplicate customer_id 10
		{"id": "ord-4", "order_number": "ORD-004", "customer_id": nil},       // null relation
	}

	batchCallCount := 0
	var requestedIDs []any

	batchFn := func(ctx context.Context, col *core.Collection, ids []any, sc *core.SecurityContext) ([]map[string]any, error) {
		batchCallCount++
		requestedIDs = ids

		// Return mock customer records
		results := []map[string]any{
			{"_internal_id": int64(10), "id": "cust-uuid-10", "name": "Acme Corp"},
			{"_internal_id": int64(20), "id": "cust-uuid-20", "name": "Globex Corp"},
		}
		return results, nil
	}

	resolver := query.NewRelationResolver(registry, batchFn)
	err := resolver.Resolve(context.Background(), parentRecords, []string{"customer"}, ordersCol, secCtx)
	if err != nil {
		t.Fatalf("resolver.Resolve() error = %v", err)
	}

	// Verify only 1 batch query was executed for the relation (no N+1!)
	if batchCallCount != 1 {
		t.Fatalf("expected 1 batch call, got %d", batchCallCount)
	}

	// Verify unique IDs collected: 10 and 20 (not 10 twice)
	if len(requestedIDs) != 2 {
		t.Fatalf("expected 2 unique IDs in batch query, got %d", len(requestedIDs))
	}

	// Verify parent records hydrated
	ord1Cust, ok := parentRecords[0]["customer"].(map[string]any)
	if !ok || ord1Cust["name"] != "Acme Corp" {
		t.Errorf("ord-1 customer not hydrated correctly: %+v", parentRecords[0])
	}

	ord2Cust, ok := parentRecords[1]["customer"].(map[string]any)
	if !ok || ord2Cust["name"] != "Globex Corp" {
		t.Errorf("ord-2 customer not hydrated correctly: %+v", parentRecords[1])
	}

	ord3Cust, ok := parentRecords[2]["customer"].(map[string]any)
	if !ok || ord3Cust["name"] != "Acme Corp" {
		t.Errorf("ord-3 customer not hydrated correctly: %+v", parentRecords[2])
	}

	if parentRecords[3]["customer"] != nil {
		t.Errorf("ord-4 null relation expected nil, got %+v", parentRecords[3]["customer"])
	}

	// Verify _internal_id was stripped
	if _, exists := ord1Cust["_internal_id"]; exists {
		t.Errorf("_internal_id should be stripped from hydrated relations")
	}
}

func TestCompiler_CompileBatchSelect(t *testing.T) {
	t.Parallel()

	registry := query.NewRegistry()
	compiler := query.NewCompiler(registry)

	col, _ := core.NewCollection(1, "customers", "Customers")
	_ = col.AddField(&core.Field{Name: "name", Type: core.FieldTypeText})

	secCtx := &core.SecurityContext{
		TenantID:      1,
		AllowedFields: []string{"name"},
	}

	// Test with int IDs
	intIDs := []any{int64(1), int64(2), int64(3)}
	qInt, err := compiler.CompileBatchSelect(col, intIDs, secCtx)
	if err != nil {
		t.Fatalf("CompileBatchSelect(intIDs) error = %v", err)
	}

	if !strings.Contains(qInt.SQL, "WHERE id = ANY($1)") {
		t.Errorf("expected WHERE id = ANY($1), got: %s", qInt.SQL)
	}
	if !strings.Contains(qInt.SQL, "tenant_id = $2") {
		t.Errorf("expected tenant_id = $2, got: %s", qInt.SQL)
	}

	// Test with UUID string IDs
	strIDs := []any{"uuid-1", "uuid-2"}
	qStr, err := compiler.CompileBatchSelect(col, strIDs, secCtx)
	if err != nil {
		t.Fatalf("CompileBatchSelect(strIDs) error = %v", err)
	}

	if !strings.Contains(qStr.SQL, "WHERE public_id = ANY($1)") {
		t.Errorf("expected WHERE public_id = ANY($1), got: %s", qStr.SQL)
	}
}
