//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uloydev/modulaengine/internal/command"
	"github.com/uloydev/modulaengine/internal/core"
	"github.com/uloydev/modulaengine/internal/database"
	"github.com/uloydev/modulaengine/internal/queue"
)

func getIntegrationDBURLs() (string, string) {
	dataURL := os.Getenv("TEST_DATABASE_URL")
	if dataURL == "" {
		dataURL = os.Getenv("DATABASE_URL")
	}

	controlURL := os.Getenv("TEST_DATABASE_CONTROL_URL")
	if controlURL == "" {
		controlURL = os.Getenv("DATABASE_CONTROL_URL")
	}
	if controlURL == "" {
		controlURL = dataURL
	}

	return dataURL, controlURL
}

func setupIntegrationPool(t *testing.T, ctx context.Context) *database.PoolManager {
	t.Helper()

	dataURL, controlURL := getIntegrationDBURLs()
	if dataURL == "" || controlURL == "" {
		t.Skip("skipping live postgres integration tests: TEST_DATABASE_URL / TEST_DATABASE_CONTROL_URL not set or unreachable")
		return nil
	}

	cfg := database.DefaultConfig(dataURL, controlURL)
	poolMgr, err := database.NewPoolManagerWithRetry(ctx, cfg, 2, 200*time.Millisecond)
	if err != nil {
		t.Skipf("skipping live postgres integration tests: TEST_DATABASE_URL / TEST_DATABASE_CONTROL_URL not set or unreachable: %v", err)
		return nil
	}

	return poolMgr
}

func TestPostgreSQL_FullIntegrationSuite(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	poolMgr := setupIntegrationPool(t, ctx)
	defer poolMgr.Close()

	// 1. Run System Catalog Migrations
	migrator := database.NewMigrator(poolMgr.ControlPool)
	if err := migrator.Run(ctx); err != nil {
		t.Fatalf("failed to run catalog migrations: %v", err)
	}

	// 2. Create Isolated Test Tenants
	var tenant1ID, tenant2ID int64
	tenantPrefix := fmt.Sprintf("itest_%d", time.Now().UnixNano())

	err := poolMgr.ControlPool.QueryRow(ctx,
		"INSERT INTO sys_tenants (name, slug) VALUES ($1, $2) RETURNING id;",
		"Tenant One", tenantPrefix+"_1",
	).Scan(&tenant1ID)
	if err != nil {
		t.Fatalf("failed to insert tenant 1: %v", err)
	}

	err = poolMgr.ControlPool.QueryRow(ctx,
		"INSERT INTO sys_tenants (name, slug) VALUES ($1, $2) RETURNING id;",
		"Tenant Two", tenantPrefix+"_2",
	).Scan(&tenant2ID)
	if err != nil {
		t.Fatalf("failed to insert tenant 2: %v", err)
	}

	// Create test users for auditing
	var user1ID, user2ID int64
	_ = poolMgr.ControlPool.QueryRow(ctx,
		"INSERT INTO sys_users (tenant_id, email, password_hash) VALUES ($1, $2, $3) RETURNING id;",
		tenant1ID, "user1@tenant1.test", "hash1",
	).Scan(&user1ID)

	_ = poolMgr.ControlPool.QueryRow(ctx,
		"INSERT INTO sys_users (tenant_id, email, password_hash) VALUES ($1, $2, $3) RETURNING id;",
		tenant2ID, "user2@tenant2.test", "hash2",
	).Scan(&user2ID)

	// 3. Compile and Execute Physical DDL for Collections
	custColName := fmt.Sprintf("cust_%d", time.Now().UnixNano()%100000)
	ordColName := fmt.Sprintf("ord_%d", time.Now().UnixNano()%100000)

	custCol, err := core.NewCollection(tenant1ID, custColName, "Customers")
	if err != nil {
		t.Fatalf("failed to create collection: %v", err)
	}
	_ = custCol.AddField(&core.Field{Name: "name", Type: core.FieldTypeText, IsRequired: true})

	ordCol, err := core.NewCollection(tenant1ID, ordColName, "Orders")
	if err != nil {
		t.Fatalf("failed to create collection: %v", err)
	}
	_ = ordCol.AddField(&core.Field{Name: "total_amount", Type: core.FieldTypeNumber})
	_ = ordCol.AddField(&core.Field{
		Name: "customer_id",
		Type: core.FieldTypeRelation,
		Relation: &core.RelationDef{
			RelationType:     core.RelationTypeM2O,
			TargetCollection: custColName,
			OnDelete:         core.OnDeleteRestrict,
		},
	})

	ddlCompiler := database.NewDDLCompiler()

	custStmts, err := ddlCompiler.CompileCreateTable(custCol)
	if err != nil {
		t.Fatalf("CompileCreateTable custCol error: %v", err)
	}
	for _, stmt := range custStmts {
		if _, err := poolMgr.ControlPool.Exec(ctx, stmt); err != nil {
			t.Fatalf("execute customer DDL statement failed: %v\nSQL: %s", err, stmt)
		}
	}

	ordStmts, err := ddlCompiler.CompileCreateTable(ordCol)
	if err != nil {
		t.Fatalf("CompileCreateTable ordCol error: %v", err)
	}
	for _, stmt := range ordStmts {
		if _, err := poolMgr.ControlPool.Exec(ctx, stmt); err != nil {
			t.Fatalf("execute order DDL statement failed: %v\nSQL: %s", err, stmt)
		}
	}

	defer func() {
		_, _ = poolMgr.ControlPool.Exec(context.Background(), fmt.Sprintf("DROP TABLE IF EXISTS %s, %s CASCADE;", ordCol.TableName, custCol.TableName))
	}()

	// 4. Insert legitimate customer for Tenant 2
	var t2CustID int64
	err = poolMgr.DataPool.QueryRow(ctx,
		fmt.Sprintf("INSERT INTO %s (tenant_id, created_by, updated_by, name) VALUES ($1, $2, $3, $4) RETURNING id;", custCol.TableName),
		tenant2ID, user2ID, user2ID, "Tenant 2 Customer",
	).Scan(&t2CustID)
	if err != nil {
		t.Fatalf("failed to insert tenant 2 customer: %v", err)
	}

	// 5. Zero-Trust Invariant: Verify Cross-Tenant FK Violation
	// Tenant 1 order attempts to reference Tenant 2's customer ID.
	// Composite FK (tenant_id, customer_id) MUST physically reject this!
	crossTenantInsertSQL := fmt.Sprintf(
		"INSERT INTO %s (tenant_id, created_by, updated_by, total_amount, customer_id) VALUES ($1, $2, $3, $4, $5);",
		ordCol.TableName,
	)
	_, err = poolMgr.DataPool.Exec(ctx, crossTenantInsertSQL, tenant1ID, user1ID, user1ID, 100.0, t2CustID)
	if err == nil {
		t.Fatalf("SECURITY VIOLATION: Database permitted cross-tenant foreign key reference!")
	}

	// 6. Test Optimistic Concurrency Control (OCC)
	executor := command.NewExecutor()
	secCtx1 := &core.SecurityContext{
		TenantID:      tenant1ID,
		UserID:        user1ID,
		AllowedFields: []string{"*"},
	}

	// Create customer for Tenant 1
	var t1CustID int64
	err = poolMgr.DataPool.QueryRow(ctx,
		fmt.Sprintf("INSERT INTO %s (tenant_id, created_by, updated_by, name) VALUES ($1, $2, $3, $4) RETURNING id;", custCol.TableName),
		tenant1ID, user1ID, user1ID, "Tenant 1 Customer",
	).Scan(&t1CustID)
	if err != nil {
		t.Fatalf("failed to insert tenant 1 customer: %v", err)
	}

	tx1, err := poolMgr.DataPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx1 error: %v", err)
	}
	createRes, err := executor.ExecuteInsert(ctx, tx1, ordCol, map[string]any{
		"total_amount": 500.0,
		"customer_id":  t1CustID,
	}, secCtx1)
	if err != nil {
		_ = tx1.Rollback(ctx)
		t.Fatalf("ExecuteInsert order error: %v", err)
	}
	if err := tx1.Commit(ctx); err != nil {
		t.Fatalf("commit tx1 error: %v", err)
	}

	// First update succeeds (version 1 -> 2)
	tx2, err := poolMgr.DataPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx2 error: %v", err)
	}
	updateRes, err := executor.ExecuteUpdate(ctx, tx2, ordCol, createRes.PublicID, 1, map[string]any{
		"total_amount": 750.0,
	}, secCtx1)
	if err != nil {
		_ = tx2.Rollback(ctx)
		t.Fatalf("ExecuteUpdate order error: %v", err)
	}
	if err := tx2.Commit(ctx); err != nil {
		t.Fatalf("commit tx2 error: %v", err)
	}
	if updateRes.Version != 2 {
		t.Errorf("expected version 2 after update, got %d", updateRes.Version)
	}

	// Second concurrent update with stale expected version 1 MUST fail with ErrVersionConflict
	tx3, err := poolMgr.DataPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx3 error: %v", err)
	}
	_, err = executor.ExecuteUpdate(ctx, tx3, ordCol, createRes.PublicID, 1, map[string]any{
		"total_amount": 999.0,
	}, secCtx1)
	_ = tx3.Rollback(ctx)

	if !errors.Is(err, core.ErrVersionConflict) {
		t.Fatalf("expected ErrVersionConflict on stale version update, got: %v", err)
	}

	// 7. Test Embedded Job Queue with Concurrent FOR UPDATE SKIP LOCKED
	workerPool := queue.NewWorkerPool(poolMgr.DataPool, queue.DefaultWorkerConfig("itest-worker"))

	numJobs := 10
	for i := 0; i < numJobs; i++ {
		_, err := workerPool.Enqueue(ctx, tenant1ID, nil, "itest:job", []byte(fmt.Sprintf(`{"index":%d}`, i)), time.Now(), 3)
		if err != nil {
			t.Fatalf("failed to enqueue job: %v", err)
		}
	}

	var claimedCount int64
	var workerWg sync.WaitGroup
	numWorkers := 4

	for w := 0; w < numWorkers; w++ {
		workerWg.Add(1)
		go func(workerID int) {
			defer workerWg.Done()
			wPool := queue.NewWorkerPool(poolMgr.DataPool, queue.DefaultWorkerConfig(fmt.Sprintf("worker-%d", workerID)))

			for {
				claimCtx, claimCancel := context.WithTimeout(ctx, 1*time.Second)
				job, err := wPool.Claim(claimCtx)
				claimCancel()

				if err != nil {
					if errors.Is(err, queue.ErrNoJobAvailable) {
						return
					}
					return
				}

				atomic.AddInt64(&claimedCount, 1)

				// Mark completed
				_, _ = poolMgr.DataPool.Exec(ctx, queue.CompleteJobSQL, job.ID)
			}
		}(w)
	}

	workerWg.Wait()

	if claimedCount != int64(numJobs) {
		t.Errorf("expected %d jobs claimed across workers, got %d", numJobs, claimedCount)
	}
}
