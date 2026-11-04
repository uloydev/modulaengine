package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/uloydev/modulaengine/internal/command"
	"github.com/uloydev/modulaengine/internal/core"
	"github.com/uloydev/modulaengine/internal/query"
)

type TxDB interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Ping(ctx context.Context) error
}

type DataHandler struct {
	db            TxDB
	registry      *query.Registry
	queryCompiler *query.Compiler
	cmdCompiler   *command.Compiler
	executor      *command.Executor
	hub           *SSEHub
}

func NewDataHandler(
	db TxDB,
	registry *query.Registry,
	queryCompiler *query.Compiler,
	cmdCompiler *command.Compiler,
	executor *command.Executor,
	hub *SSEHub,
) *DataHandler {
	return &DataHandler{
		db:            db,
		registry:      registry,
		queryCompiler: queryCompiler,
		cmdCompiler:   cmdCompiler,
		executor:      executor,
		hub:           hub,
	}
}

func (h *DataHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		RespondError(w, r, http.StatusServiceUnavailable, "Database Unavailable", "Database connection pool not configured")
		return
	}

	secCtx, ok := GetSecurityContext(r.Context())
	if !ok || secCtx == nil {
		RespondError(w, r, http.StatusUnauthorized, "Unauthorized", "Security context required")
		return
	}

	colName := chi.URLParam(r, "collection")
	col, exists := h.registry.GetCollection(secCtx.TenantID, colName)
	if !exists {
		RespondError(w, r, http.StatusNotFound, "Not Found", fmt.Sprintf("Collection '%s' not found", colName))
		return
	}

	params, err := query.ParseQueryParams(r.URL.Query())
	if err != nil {
		RespondError(w, r, http.StatusBadRequest, "Bad Request", err.Error())
		return
	}

	compiled, err := h.queryCompiler.CompileSelect(col, params, secCtx)
	if err != nil {
		if errors.Is(err, query.ErrFieldNotPermitted) {
			RespondError(w, r, http.StatusForbidden, "Forbidden", err.Error())
			return
		}
		if errors.Is(err, query.ErrUnknownField) {
			RespondError(w, r, http.StatusBadRequest, "Bad Request", err.Error())
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}

	rows, err := h.db.Query(r.Context(), compiled.SQL, compiled.Args...)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "Database Query Error", err.Error())
		return
	}
	defer rows.Close()

	records, err := scanRowsToMaps(rows, compiled.Columns)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "Row Serialization Error", err.Error())
		return
	}

	response := map[string]any{
		"data": records,
		"meta": map[string]any{
			"limit":  params.Limit,
			"offset": params.Offset,
			"count":  len(records),
		},
	}

	RespondJSON(w, http.StatusOK, response)
}

func (h *DataHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		RespondError(w, r, http.StatusServiceUnavailable, "Database Unavailable", "Database connection pool not configured")
		return
	}

	secCtx, ok := GetSecurityContext(r.Context())
	if !ok || secCtx == nil {
		RespondError(w, r, http.StatusUnauthorized, "Unauthorized", "Security context required")
		return
	}

	colName := chi.URLParam(r, "collection")
	publicID := chi.URLParam(r, "id")

	col, exists := h.registry.GetCollection(secCtx.TenantID, colName)
	if !exists {
		RespondError(w, r, http.StatusNotFound, "Not Found", fmt.Sprintf("Collection '%s' not found", colName))
		return
	}

	params := &query.QueryParams{
		Limit:  1,
		Offset: 0,
		Filter: &query.ComparisonNode{
			Field: "public_id",
			Op:    query.OpEq,
			Value: publicID,
		},
	}

	compiled, err := h.queryCompiler.CompileSelect(col, params, secCtx)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "Internal Server Error", err.Error())
		return
	}

	rows, err := h.db.Query(r.Context(), compiled.SQL, compiled.Args...)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "Database Query Error", err.Error())
		return
	}
	defer rows.Close()

	records, err := scanRowsToMaps(rows, compiled.Columns)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "Row Serialization Error", err.Error())
		return
	}

	if len(records) == 0 {
		RespondError(w, r, http.StatusNotFound, "Not Found", fmt.Sprintf("Record '%s' not found", publicID))
		return
	}

	RespondJSON(w, http.StatusOK, map[string]any{"data": records[0]})
}

func (h *DataHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		RespondError(w, r, http.StatusServiceUnavailable, "Database Unavailable", "Database connection pool not configured")
		return
	}

	secCtx, ok := GetSecurityContext(r.Context())
	if !ok || secCtx == nil {
		RespondError(w, r, http.StatusUnauthorized, "Unauthorized", "Security context required")
		return
	}

	colName := chi.URLParam(r, "collection")
	col, exists := h.registry.GetCollection(secCtx.TenantID, colName)
	if !exists {
		RespondError(w, r, http.StatusNotFound, "Not Found", fmt.Sprintf("Collection '%s' not found", colName))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 10*1024*1024)
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		RespondError(w, r, http.StatusBadRequest, "Bad Request", "Invalid JSON body")
		return
	}

	tx, err := h.db.Begin(r.Context())
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "Transaction Error", err.Error())
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	result, err := h.executor.ExecuteInsert(r.Context(), tx, col, payload, secCtx)
	if err != nil {
		if errors.Is(err, query.ErrFieldNotPermitted) {
			RespondError(w, r, http.StatusForbidden, "Forbidden", err.Error())
			return
		}
		if errors.Is(err, core.ErrValidation) || errors.Is(err, query.ErrUnknownField) {
			RespondError(w, r, http.StatusBadRequest, "Validation Error", err.Error())
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "Insert Failed", err.Error())
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		RespondError(w, r, http.StatusInternalServerError, "Transaction Commit Error", err.Error())
		return
	}

	if h.hub != nil {
		h.hub.Broadcast(&EventMessage{
			Collection: col.Name,
			Action:     "INSERT",
			TenantID:   secCtx.TenantID,
			PublicID:   result.PublicID,
			Changes:    payload,
			Version:    result.Version,
			UpdatedAt:  result.CreatedAt,
		})
	}

	RespondJSON(w, http.StatusCreated, map[string]any{"data": result})
}

func (h *DataHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		RespondError(w, r, http.StatusServiceUnavailable, "Database Unavailable", "Database connection pool not configured")
		return
	}

	secCtx, ok := GetSecurityContext(r.Context())
	if !ok || secCtx == nil {
		RespondError(w, r, http.StatusUnauthorized, "Unauthorized", "Security context required")
		return
	}

	colName := chi.URLParam(r, "collection")
	publicID := chi.URLParam(r, "id")

	col, exists := h.registry.GetCollection(secCtx.TenantID, colName)
	if !exists {
		RespondError(w, r, http.StatusNotFound, "Not Found", fmt.Sprintf("Collection '%s' not found", colName))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 10*1024*1024)
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		RespondError(w, r, http.StatusBadRequest, "Bad Request", "Invalid JSON body")
		return
	}

	// Resolve version from body or If-Match header
	version := 0
	if v, exists := payload["version"]; exists {
		if floatVal, ok := v.(float64); ok {
			version = int(floatVal)
		} else if intVal, ok := v.(int); ok {
			version = intVal
		}
		delete(payload, "version")
	} else if ifMatch := r.Header.Get("If-Match"); ifMatch != "" {
		parsed, _ := strconv.Atoi(ifMatch)
		version = parsed
	}

	if version <= 0 {
		RespondError(w, r, http.StatusBadRequest, "Bad Request", "Missing or invalid version parameter for concurrency check")
		return
	}

	tx, err := h.db.Begin(r.Context())
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "Transaction Error", err.Error())
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	result, err := h.executor.ExecuteUpdate(r.Context(), tx, col, publicID, version, payload, secCtx)
	if err != nil {
		if errors.Is(err, core.ErrVersionConflict) {
			RespondError(w, r, http.StatusConflict, "Optimistic Concurrency Conflict", err.Error())
			return
		}
		if errors.Is(err, core.ErrNotFound) {
			RespondError(w, r, http.StatusNotFound, "Not Found", fmt.Sprintf("Record '%s' not found", publicID))
			return
		}
		if errors.Is(err, query.ErrFieldNotPermitted) {
			RespondError(w, r, http.StatusForbidden, "Forbidden", err.Error())
			return
		}
		if errors.Is(err, core.ErrValidation) || errors.Is(err, query.ErrUnknownField) {
			RespondError(w, r, http.StatusBadRequest, "Validation Error", err.Error())
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "Update Failed", err.Error())
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		RespondError(w, r, http.StatusInternalServerError, "Transaction Commit Error", err.Error())
		return
	}

	if h.hub != nil {
		h.hub.Broadcast(&EventMessage{
			Collection: col.Name,
			Action:     "UPDATE",
			TenantID:   secCtx.TenantID,
			PublicID:   result.PublicID,
			Changes:    payload,
			Version:    result.Version,
			UpdatedAt:  result.UpdatedAt,
		})
	}

	RespondJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (h *DataHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		RespondError(w, r, http.StatusServiceUnavailable, "Database Unavailable", "Database connection pool not configured")
		return
	}

	secCtx, ok := GetSecurityContext(r.Context())
	if !ok || secCtx == nil {
		RespondError(w, r, http.StatusUnauthorized, "Unauthorized", "Security context required")
		return
	}

	colName := chi.URLParam(r, "collection")
	publicID := chi.URLParam(r, "id")

	col, exists := h.registry.GetCollection(secCtx.TenantID, colName)
	if !exists {
		RespondError(w, r, http.StatusNotFound, "Not Found", fmt.Sprintf("Collection '%s' not found", colName))
		return
	}

	version := 1
	if vStr := r.URL.Query().Get("version"); vStr != "" {
		if parsed, err := strconv.Atoi(vStr); err == nil && parsed > 0 {
			version = parsed
		}
	} else if ifMatch := r.Header.Get("If-Match"); ifMatch != "" {
		if parsed, err := strconv.Atoi(ifMatch); err == nil && parsed > 0 {
			version = parsed
		}
	}

	tx, err := h.db.Begin(r.Context())
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "Transaction Error", err.Error())
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()

	result, err := h.executor.ExecuteSoftDelete(r.Context(), tx, col, publicID, version, secCtx)
	if err != nil {
		if errors.Is(err, core.ErrVersionConflict) {
			RespondError(w, r, http.StatusConflict, "Optimistic Concurrency Conflict", err.Error())
			return
		}
		if errors.Is(err, core.ErrNotFound) {
			RespondError(w, r, http.StatusNotFound, "Not Found", fmt.Sprintf("Record '%s' not found", publicID))
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "Delete Failed", err.Error())
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		RespondError(w, r, http.StatusInternalServerError, "Transaction Commit Error", err.Error())
		return
	}

	if h.hub != nil {
		h.hub.Broadcast(&EventMessage{
			Collection: col.Name,
			Action:     "DELETE",
			TenantID:   secCtx.TenantID,
			PublicID:   result.PublicID,
			Version:    result.Version,
			UpdatedAt:  time.Now(),
		})
	}

	w.WriteHeader(http.StatusNoContent)
}

func scanRowsToMaps(rows pgx.Rows, columns []string) ([]map[string]any, error) {
	results := make([]map[string]any, 0, 16)
	fieldDescs := rows.FieldDescriptions()
	colNames := make([]string, len(fieldDescs))
	for i, fd := range fieldDescs {
		colNames[i] = fd.Name
	}

	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}

		record := make(map[string]any, len(colNames))
		for i, name := range colNames {
			val := values[i]
			record[name] = val
		}
		results = append(results, record)
	}

	return results, rows.Err()
}
