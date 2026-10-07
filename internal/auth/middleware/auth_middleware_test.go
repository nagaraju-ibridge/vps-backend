package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"vpsmonitoring-backend/internal/auth/middleware"
	"vpsmonitoring-backend/internal/auth/service"
)

type mockAuthService struct {
	service.AuthService
	validateCalls int
}

func (m *mockAuthService) ValidateToken(_ string) (*service.UserClaims, error) {
	m.validateCalls++
	return nil, service.ErrInvalidToken
}

func TestJWTRoleMiddleware_AgentRoutesBypassJWT(t *testing.T) {
	tests := []struct {
		name              string
		path              string
		wantStatus        int
		wantValidateCalls int
	}{
		{
			name:              "heartbeat agent route bypasses JWT middleware",
			path:              "/api/v1/agent/heartbeat",
			wantStatus:        http.StatusOK,
			wantValidateCalls: 0,
		},
		{
			name:              "metrics agent route bypasses JWT middleware",
			path:              "/api/v1/agent/metrics",
			wantStatus:        http.StatusOK,
			wantValidateCalls: 0,
		},
		{
			name:              "process plural agent route bypasses JWT middleware",
			path:              "/api/v1/agents/agent-123/processes",
			wantStatus:        http.StatusOK,
			wantValidateCalls: 0,
		},
		{
			name:              "normal user route does not bypass JWT middleware",
			path:              "/api/v1/user/servers",
			wantStatus:        http.StatusUnauthorized,
			wantValidateCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authSvc := &mockAuthService{}
			handler := middleware.JWTRoleMiddleware(authSvc)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			req.Header.Set("Authorization", "Bearer raw-agent-or-user-token")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d", tt.wantStatus, rec.Code)
			}
			if authSvc.validateCalls != tt.wantValidateCalls {
				t.Fatalf("expected ValidateToken calls %d, got %d", tt.wantValidateCalls, authSvc.validateCalls)
			}
		})
	}
}
