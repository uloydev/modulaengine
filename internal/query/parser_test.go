package query_test

import (
	"errors"
	"net/url"
	"testing"

	"github.com/uloydev/modulaengine/internal/query"
)

func TestParseQueryParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		queryStr   string
		wantLimit  int
		wantOffset int
		wantSort   int
		wantFields int
		wantErr    bool
		errIs      error
	}{
		{
			name:       "default params",
			queryStr:   "",
			wantLimit:  25,
			wantOffset: 0,
			wantSort:   0,
			wantFields: 0,
			wantErr:    false,
		},
		{
			name:       "custom pagination and sort",
			queryStr:   "limit=50&offset=100&sort=-created_at,order_number",
			wantLimit:  50,
			wantOffset: 100,
			wantSort:   2,
			wantFields: 0,
			wantErr:    false,
		},
		{
			name:       "field projection and filter",
			queryStr:   "fields=title,price,status&filter[status][_eq]=PAID&filter[price][_gt]=5000",
			wantLimit:  25,
			wantOffset: 0,
			wantSort:   0,
			wantFields: 3,
			wantErr:    false,
		},
		{
			name:       "in filter and null filter",
			queryStr:   "filter[status][_in]=DRAFT,PAID&filter[deleted_at][_null]=true",
			wantLimit:  25,
			wantOffset: 0,
			wantSort:   0,
			wantFields: 0,
			wantErr:    false,
		},
		{
			name:     "invalid limit",
			queryStr: "limit=invalid",
			wantErr:  true,
			errIs:    query.ErrInvalidFilterNode,
		},
		{
			name:     "invalid offset",
			queryStr: "offset=-5",
			wantErr:  true,
			errIs:    query.ErrInvalidFilterNode,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			vals, err := url.ParseQuery(tt.queryStr)
			if err != nil {
				t.Fatalf("unexpected url.ParseQuery error: %v", err)
			}

			qp, err := query.ParseQueryParams(vals)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseQueryParams() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if tt.errIs != nil && !errors.Is(err, tt.errIs) {
					t.Fatalf("expected error %v, got %v", tt.errIs, err)
				}
				return
			}

			if qp.Limit != tt.wantLimit {
				t.Errorf("Limit = %d, want %d", qp.Limit, tt.wantLimit)
			}
			if qp.Offset != tt.wantOffset {
				t.Errorf("Offset = %d, want %d", qp.Offset, tt.wantOffset)
			}
			if len(qp.Sort) != tt.wantSort {
				t.Errorf("len(Sort) = %d, want %d", len(qp.Sort), tt.wantSort)
			}
			if len(qp.Fields) != tt.wantFields {
				t.Errorf("len(Fields) = %d, want %d", len(qp.Fields), tt.wantFields)
			}
		})
	}
}

func TestASTValidation_DepthLimit(t *testing.T) {
	t.Parallel()

	// Build deep 6-level AST
	var root query.Node = &query.ComparisonNode{
		Field: "status",
		Op:    query.OpEq,
		Value: "ACTIVE",
	}

	for i := 0; i < 6; i++ {
		root = &query.LogicalNode{
			Op:       query.LogicalAnd,
			Children: []query.Node{root},
		}
	}

	err := root.Validate(1)
	if !errors.Is(err, query.ErrDepthExceeded) {
		t.Fatalf("expected ErrDepthExceeded, got %v", err)
	}
}

func TestASTValidation_ArraySizeLimit(t *testing.T) {
	t.Parallel()

	largeArray := make([]any, 501)
	for i := range largeArray {
		largeArray[i] = i
	}

	node := &query.ComparisonNode{
		Field: "id",
		Op:    query.OpIn,
		Value: largeArray,
	}

	err := node.Validate(1)
	if !errors.Is(err, query.ErrArraySizeExceeded) {
		t.Fatalf("expected ErrArraySizeExceeded, got %v", err)
	}
}
