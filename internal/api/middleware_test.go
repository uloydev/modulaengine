package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/uloydev/modulaengine/internal/api"
	"github.com/uloydev/modulaengine/internal/core"
)

func TestJWTAuthMiddleware(t *testing.T) {
	t.Parallel()

	secret := []byte("super-secret-test-key-32bytes-long!!")
	authMiddleware := api.JWTAuthMiddleware(secret)

	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secCtx, ok := api.GetSecurityContext(r.Context())
		if !ok || secCtx == nil {
			t.Errorf("expected security context in request context")
		}
		w.WriteHeader(http.StatusOK)
	})

	protected := authMiddleware(dummyHandler)

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{
			name:       "missing header",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "invalid bearer format",
			authHeader: "Basic 123456",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "invalid token signature",
			authHeader: "Bearer invalid.token.payload",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "valid token",
			authHeader: func() string {
				secCtx := &core.SecurityContext{
					TenantID:      100,
					UserID:        42,
					RoleID:        2,
					RoleName:      "sales_rep",
					AllowedFields: []string{"title", "amount"},
				}
				token, _ := api.GenerateTestToken(secret, secCtx, 15*time.Minute)
				return "Bearer " + token
			}(),
			wantStatus: http.StatusOK,
		},
		{
			name: "expired token",
			authHeader: func() string {
				secCtx := &core.SecurityContext{
					TenantID: 100,
					UserID:   42,
				}
				token, _ := api.GenerateTestToken(secret, secCtx, -1*time.Minute)
				return "Bearer " + token
			}(),
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/api/data/orders", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()

			protected.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}
