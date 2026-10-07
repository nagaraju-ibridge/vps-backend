package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"gofr.dev/pkg/gofr"

	"vpsmonitoring-backend/internal/auth/models"
	"vpsmonitoring-backend/internal/auth/service"
)

type contextKey string

const (
	UserClaimsContextKey contextKey = "vpspulse_user_claims"
)

// JWTRoleMiddleware enforces CORS, JWT authentication, and role-based route protection
func JWTRoleMiddleware(authService service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. CORS headers
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, X-Requested-With")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}

			path := r.URL.Path

			// 2. Allow public endpoints without auth
			if path == "/api/v1/auth/login" ||
				path == "/api/v1/auth/register" ||
				strings.HasPrefix(path, "/api/v1/agent/") ||
				strings.HasPrefix(path, "/api/v1/agents/") ||
				path == "/check-health" ||
				path == "/health" ||
				path == "/api/db-status" ||
				strings.HasPrefix(path, "/.well-known") ||
				path == "/metrics" {
				next.ServeHTTP(w, r)
				return
			}

			// 3. Only guard /api/v1 routes
			if !strings.HasPrefix(path, "/api/v1/") {
				next.ServeHTTP(w, r)
				return
			}

			// 4. Check Authorization Header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				respondJSONError(w, http.StatusUnauthorized, "Authorization header missing")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
				respondJSONError(w, http.StatusUnauthorized, "Invalid authorization header format, expected 'Bearer <token>'")
				return
			}
			tokenStr := strings.TrimSpace(parts[1])

			// 5. Validate token via AuthService
			claims, err := authService.ValidateToken(tokenStr)
			if err != nil {
				respondJSONError(w, http.StatusUnauthorized, "Invalid or expired token")
				return
			}

			// 6. Role-Based Route Authorization
			if strings.HasPrefix(path, "/api/v1/admin") && claims.Role != models.RoleAdmin {
				respondJSONError(w, http.StatusForbidden, "Forbidden: ADMIN role required to access this endpoint")
				return
			}

			if strings.HasPrefix(path, "/api/v1/user") && claims.Role != models.RoleUser && claims.Role != models.RoleAdmin {
				respondJSONError(w, http.StatusForbidden, "Forbidden: USER role required to access this endpoint")
				return
			}

			// 7. Inject claims into request context
			ctx := context.WithValue(r.Context(), UserClaimsContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetClaims retrieves UserClaims from the GoFr context
func GetClaims(ctx *gofr.Context) (*service.UserClaims, error) {
	if ctx == nil {
		return nil, errors.New("context is nil")
	}

	val := ctx.Context.Value(UserClaimsContextKey)
	if val == nil {
		return nil, errors.New("unauthenticated request: claims not found in context")
	}

	claims, ok := val.(*service.UserClaims)
	if !ok || claims == nil {
		return nil, errors.New("invalid claims format in context")
	}

	return claims, nil
}

func respondJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(fmt.Sprintf(`{"error":{"message":"%s"}}`, message)))
}
