package command

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/uloydev/modulaengine/internal/core"
)

type Executor struct {
	compiler *Compiler
}

func NewExecutor() *Executor {
	return &Executor{
		compiler: NewCompiler(),
	}
}

func (e *Executor) ExecuteInsert(
	ctx context.Context,
	tx pgx.Tx,
	col *core.Collection,
	data map[string]any,
	secCtx *core.SecurityContext,
) (*MutationResult, error) {
	compiled, err := e.compiler.CompileInsert(col, data, secCtx)
	if err != nil {
		return nil, fmt.Errorf("compile insert: %w", err)
	}

	var (
		internalID int64
		publicID   string
		version    int
		createdAt  time.Time
		updatedAt  time.Time
	)

	row := tx.QueryRow(ctx, compiled.SQL, compiled.Args...)
	err = row.Scan(&internalID, &publicID, &version, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("execute insert: %w", err)
	}

	diffJSON, err := json.Marshal(compiled.FieldDiff)
	if err != nil {
		return nil, fmt.Errorf("marshal audit diff: %w", err)
	}

	auditSQL := `INSERT INTO sys_audit_logs (tenant_id, collection_id, record_id, action, user_id, diff)
VALUES ($1, $2, $3, $4, $5, $6);`
	_, err = tx.Exec(ctx, auditSQL, secCtx.TenantID, col.ID, internalID, ActionInsert, secCtx.UserID, diffJSON)
	if err != nil {
		return nil, fmt.Errorf("insert audit log: %w", err)
	}

	return &MutationResult{
		ID:        internalID,
		PublicID:  publicID,
		Version:   version,
		Diff:      compiled.FieldDiff,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

func (e *Executor) ExecuteUpdate(
	ctx context.Context,
	tx pgx.Tx,
	col *core.Collection,
	publicID string,
	expectedVersion int,
	data map[string]any,
	secCtx *core.SecurityContext,
) (*MutationResult, error) {
	// 1. Lock existing row, retrieve current state and check version
	checkSQL := fmt.Sprintf(
		"SELECT id, version, to_jsonb(t) FROM %s t WHERE public_id = $1 AND tenant_id = $2 AND deleted_at IS NULL FOR UPDATE;",
		col.TableName,
	)

	var (
		internalID     int64
		currentVersion int
		rawOldJSON     []byte
	)
	err := tx.QueryRow(ctx, checkSQL, publicID, secCtx.TenantID).Scan(&internalID, &currentVersion, &rawOldJSON)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("lock existing record: %w", err)
	}

	if currentVersion != expectedVersion {
		return nil, fmt.Errorf("%w: expected %d, got %d", core.ErrVersionConflict, expectedVersion, currentVersion)
	}

	var oldValues map[string]any
	if len(rawOldJSON) > 0 {
		if err := json.Unmarshal(rawOldJSON, &oldValues); err != nil {
			return nil, fmt.Errorf("unmarshal existing record state: %w", err)
		}
	}

	// 2. Compile and execute update
	compiled, err := e.compiler.CompileUpdate(col, publicID, expectedVersion, data, secCtx)
	if err != nil {
		return nil, fmt.Errorf("compile update: %w", err)
	}

	var (
		resInternalID int64
		resPublicID   string
		resVersion    int
		updatedAt     time.Time
	)

	err = tx.QueryRow(ctx, compiled.SQL, compiled.Args...).Scan(&resInternalID, &resPublicID, &resVersion, &updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core.ErrVersionConflict
		}
		return nil, fmt.Errorf("execute update: %w", err)
	}

	realDiff := ComputeDiff(oldValues, data)
	diffJSON, err := json.Marshal(realDiff)
	if err != nil {
		return nil, fmt.Errorf("marshal audit diff: %w", err)
	}

	auditSQL := `INSERT INTO sys_audit_logs (tenant_id, collection_id, record_id, action, user_id, diff)
VALUES ($1, $2, $3, $4, $5, $6);`
	_, err = tx.Exec(ctx, auditSQL, secCtx.TenantID, col.ID, resInternalID, ActionUpdate, secCtx.UserID, diffJSON)
	if err != nil {
		return nil, fmt.Errorf("insert audit log: %w", err)
	}

	return &MutationResult{
		ID:        resInternalID,
		PublicID:  resPublicID,
		Version:   resVersion,
		Diff:      realDiff,
		UpdatedAt: updatedAt,
	}, nil
}

func (e *Executor) ExecuteSoftDelete(
	ctx context.Context,
	tx pgx.Tx,
	col *core.Collection,
	publicID string,
	expectedVersion int,
	secCtx *core.SecurityContext,
) (*MutationResult, error) {
	// 1. Lock existing row and check current version
	checkSQL := fmt.Sprintf(
		"SELECT id, version FROM %s WHERE public_id = $1 AND tenant_id = $2 AND deleted_at IS NULL FOR UPDATE;",
		col.TableName,
	)

	var (
		internalID     int64
		currentVersion int
	)
	err := tx.QueryRow(ctx, checkSQL, publicID, secCtx.TenantID).Scan(&internalID, &currentVersion)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("lock existing record for delete: %w", err)
	}

	if currentVersion != expectedVersion {
		return nil, fmt.Errorf("%w: expected %d, got %d", core.ErrVersionConflict, expectedVersion, currentVersion)
	}

	// 2. Compile and execute soft delete
	compiled, err := e.compiler.CompileSoftDelete(col, publicID, expectedVersion, secCtx)
	if err != nil {
		return nil, fmt.Errorf("compile soft delete: %w", err)
	}

	var (
		resInternalID int64
		resPublicID   string
		resVersion    int
		deletedAt     time.Time
	)

	err = tx.QueryRow(ctx, compiled.SQL, compiled.Args...).Scan(&resInternalID, &resPublicID, &resVersion, &deletedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core.ErrVersionConflict
		}
		return nil, fmt.Errorf("execute soft delete: %w", err)
	}

	diffJSON, _ := json.Marshal(map[string]any{"deleted": true})
	auditSQL := `INSERT INTO sys_audit_logs (tenant_id, collection_id, record_id, action, user_id, diff)
VALUES ($1, $2, $3, $4, $5, $6);`
	_, err = tx.Exec(ctx, auditSQL, secCtx.TenantID, col.ID, resInternalID, ActionDelete, secCtx.UserID, diffJSON)
	if err != nil {
		return nil, fmt.Errorf("insert audit log: %w", err)
	}

	return &MutationResult{
		ID:       resInternalID,
		PublicID: resPublicID,
		Version:  resVersion,
	}, nil
}
