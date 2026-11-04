package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/uloydev/modulaengine/internal/core"
)

type contextKey string

const (
	securityContextKey contextKey = "security_context"
)

func WithSecurityContext(ctx context.Context, secCtx *core.SecurityContext) context.Context {
	return context.WithValue(ctx, securityContextKey, secCtx)
}

func GetSecurityContext(ctx context.Context) (*core.SecurityContext, bool) {
	val := ctx.Value(securityContextKey)
	if val == nil {
		return nil, false
	}
	secCtx, ok := val.(*core.SecurityContext)
	return secCtx, ok
}

type CustomClaims struct {
	TenantID      int64    `json:"tenant_id"`
	UserID        int64    `json:"user_id"`
	RoleID        int64    `json:"role_id"`
	RoleName      string   `json:"role_name"`
	IsSuperAdmin  bool     `json:"is_super_admin"`
	AllowedFields []string `json:"allowed_fields"`
	RLSFilter     string   `json:"rls_filter,omitempty"`
	jwt.RegisteredClaims
}

func JWTAuthMiddleware(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				RespondError(w, r, http.StatusUnauthorized, "Unauthorized", "Missing Authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				RespondError(w, r, http.StatusUnauthorized, "Unauthorized", "Authorization header format must be Bearer <token>")
				return
			}

			tokenStr := parts[1]
			claims := &CustomClaims{}
			token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
				return secret, nil
			})

			if err != nil || !token.Valid {
				RespondError(w, r, http.StatusUnauthorized, "Unauthorized", "Invalid or expired authorization token")
				return
			}

			if claims.TenantID <= 0 {
				RespondError(w, r, http.StatusUnauthorized, "Unauthorized", "Token missing valid tenant identity")
				return
			}

			secCtx := &core.SecurityContext{
				TenantID:      claims.TenantID,
				UserID:        claims.UserID,
				RoleID:        claims.RoleID,
				RoleName:      claims.RoleName,
				IsSuperAdmin:  claims.IsSuperAdmin,
				AllowedFields: claims.AllowedFields,
				RLSFilter:     claims.RLSFilter,
			}

			ctx := WithSecurityContext(r.Context(), secCtx)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GenerateTestToken(secret []byte, secCtx *core.SecurityContext, duration time.Duration) (string, error) {
	claims := CustomClaims{
		TenantID:      secCtx.TenantID,
		UserID:        secCtx.UserID,
		RoleID:        secCtx.RoleID,
		RoleName:      secCtx.RoleName,
		IsSuperAdmin:  secCtx.IsSuperAdmin,
		AllowedFields: secCtx.AllowedFields,
		RLSFilter:     secCtx.RLSFilter,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}
