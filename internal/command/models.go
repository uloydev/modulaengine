package command

import (
	"time"
)

type AuditAction string

const (
	ActionInsert  AuditAction = "INSERT"
	ActionUpdate  AuditAction = "UPDATE"
	ActionDelete  AuditAction = "DELETE"
	ActionRestore AuditAction = "RESTORE"
)

type AuditLog struct {
	ID           int64       `json:"id"`
	TenantID     int64       `json:"tenant_id"`
	CollectionID int64       `json:"collection_id"`
	RecordID     int64       `json:"record_id"`
	Action       AuditAction `json:"action"`
	UserID       *int64      `json:"user_id,omitempty"`
	Diff         AuditDiff   `json:"diff"`
	CreatedAt    time.Time   `json:"created_at"`
}

type MutationResult struct {
	ID        int64          `json:"-"`
	PublicID  string         `json:"id"`
	Version   int            `json:"version"`
	Values    map[string]any `json:"values,omitempty"`
	Diff      AuditDiff      `json:"diff,omitempty"`
	CreatedAt time.Time      `json:"created_at,omitempty"`
	UpdatedAt time.Time      `json:"updated_at,omitempty"`
}

type CompiledMutation struct {
	SQL       string
	Args      []any
	Action    AuditAction
	Columns   []string
	FieldDiff AuditDiff
}
