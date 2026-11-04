package api_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uloydev/modulaengine/internal/api"
	"github.com/uloydev/modulaengine/internal/core"
	"github.com/uloydev/modulaengine/internal/query"
)

func TestMapDomainError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantTitle  string
	}{
		{
			name:       "not found error",
			err:        core.ErrNotFound,
			wantStatus: http.StatusNotFound,
			wantTitle:  "Not Found",
		},
		{
			name:       "wrapped not found error",
			err:        fmt.Errorf("item lookup: %w", core.ErrNotFound),
			wantStatus: http.StatusNotFound,
			wantTitle:  "Not Found",
		},
		{
			name:       "version conflict error",
			err:        core.ErrVersionConflict,
			wantStatus: http.StatusConflict,
			wantTitle:  "Optimistic Concurrency Conflict",
		},
		{
			name:       "wrapped version conflict error",
			err:        fmt.Errorf("execute update: %w", core.ErrVersionConflict),
			wantStatus: http.StatusConflict,
			wantTitle:  "Optimistic Concurrency Conflict",
		},
		{
			name:       "field not permitted error",
			err:        query.ErrFieldNotPermitted,
			wantStatus: http.StatusForbidden,
			wantTitle:  "Forbidden",
		},
		{
			name:       "tenant mismatch error",
			err:        core.ErrTenantMismatch,
			wantStatus: http.StatusUnauthorized,
			wantTitle:  "Unauthorized",
		},
		{
			name:       "validation error",
			err:        core.ErrValidation,
			wantStatus: http.StatusBadRequest,
			wantTitle:  "Validation Error",
		},
		{
			name:       "invalid identifier error",
			err:        core.ErrInvalidIdentifier,
			wantStatus: http.StatusBadRequest,
			wantTitle:  "Validation Error",
		},
		{
			name:       "reserved name error",
			err:        core.ErrReservedName,
			wantStatus: http.StatusBadRequest,
			wantTitle:  "Validation Error",
		},
		{
			name:       "unknown field error",
			err:        query.ErrUnknownField,
			wantStatus: http.StatusBadRequest,
			wantTitle:  "Validation Error",
		},
		{
			name:       "invalid filter node error",
			err:        query.ErrInvalidFilterNode,
			wantStatus: http.StatusBadRequest,
			wantTitle:  "Validation Error",
		},
		{
			name:       "depth exceeded error",
			err:        query.ErrDepthExceeded,
			wantStatus: http.StatusBadRequest,
			wantTitle:  "Validation Error",
		},
		{
			name:       "array size exceeded error",
			err:        query.ErrArraySizeExceeded,
			wantStatus: http.StatusBadRequest,
			wantTitle:  "Validation Error",
		},
		{
			name:       "unsupported op error",
			err:        query.ErrUnsupportedOp,
			wantStatus: http.StatusBadRequest,
			wantTitle:  "Validation Error",
		},
		{
			name:       "unknown internal error",
			err:        errors.New("unexpected database partition failure"),
			wantStatus: http.StatusInternalServerError,
			wantTitle:  "Internal Server Error",
		},
		{
			name:       "nil error fallback",
			err:        nil,
			wantStatus: http.StatusInternalServerError,
			wantTitle:  "Internal Server Error",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotStatus, gotTitle := api.MapDomainError(tt.err)
			if gotStatus != tt.wantStatus {
				t.Errorf("status = %d, want %d", gotStatus, tt.wantStatus)
			}
			if gotTitle != tt.wantTitle {
				t.Errorf("title = %q, want %q", gotTitle, tt.wantTitle)
			}
		})
	}
}

func TestRespondDomainError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		path       string
		wantStatus int
		wantTitle  string
		wantDetail string
	}{
		{
			name:       "not found response",
			err:        fmt.Errorf("%w: record '018e-test'", core.ErrNotFound),
			path:       "/api/data/orders/018e-test",
			wantStatus: http.StatusNotFound,
			wantTitle:  "Not Found",
			wantDetail: "record not found: record '018e-test'",
		},
		{
			name:       "version conflict response",
			err:        core.ErrVersionConflict,
			path:       "/api/data/deals/123",
			wantStatus: http.StatusConflict,
			wantTitle:  "Optimistic Concurrency Conflict",
			wantDetail: core.ErrVersionConflict.Error(),
		},
		{
			name:       "field forbidden response",
			err:        query.ErrFieldNotPermitted,
			path:       "/api/data/deals",
			wantStatus: http.StatusForbidden,
			wantTitle:  "Forbidden",
			wantDetail: query.ErrFieldNotPermitted.Error(),
		},
		{
			name:       "validation error response",
			err:        query.ErrUnknownField,
			path:       "/api/data/deals",
			wantStatus: http.StatusBadRequest,
			wantTitle:  "Validation Error",
			wantDetail: query.ErrUnknownField.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			api.RespondDomainError(rec, req, tt.err)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}

			contentType := rec.Header().Get("Content-Type")
			if contentType != "application/problem+json" {
				t.Errorf("Content-Type = %q, want application/problem+json", contentType)
			}

			var problem api.ProblemDetails
			if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
				t.Fatalf("failed to decode problem details JSON: %v", err)
			}

			if problem.Status != tt.wantStatus {
				t.Errorf("problem.Status = %d, want %d", problem.Status, tt.wantStatus)
			}
			if problem.Title != tt.wantTitle {
				t.Errorf("problem.Title = %q, want %q", problem.Title, tt.wantTitle)
			}
			if problem.Detail != tt.wantDetail {
				t.Errorf("problem.Detail = %q, want %q", problem.Detail, tt.wantDetail)
			}
			if problem.Instance != tt.path {
				t.Errorf("problem.Instance = %q, want %q", problem.Instance, tt.path)
			}
		})
	}
}
