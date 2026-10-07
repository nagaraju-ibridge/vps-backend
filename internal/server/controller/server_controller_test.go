package controller_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"vpsmonitoring-backend/internal/auth/middleware"
	authService "vpsmonitoring-backend/internal/auth/service"
	"vpsmonitoring-backend/internal/server/dto"
	"vpsmonitoring-backend/internal/server/service"
)

// mockServerService implements service.ServerService for controller testing
type mockServerService struct{}

func (m *mockServerService) CreateServer(ctx context.Context, userID int64, req dto.CreateServerRequest) (*dto.ServerResponse, error) {
	return &dto.ServerResponse{
		ID:     1,
		UserID: userID,
		Name:   req.Name,
		Status: "PENDING",
	}, nil
}

func (m *mockServerService) GetUserServers(ctx context.Context, userID int64) ([]dto.ServerResponse, error) {
	return []dto.ServerResponse{
		{
			ID:     1,
			UserID: userID,
			Name:   "Test Server",
			Status: "PENDING",
		},
	}, nil
}

func (m *mockServerService) GetServer(ctx context.Context, id int64, userID int64) (*dto.ServerResponse, error) {
	if id != 1 {
		return nil, service.ErrServerNotFound
	}
	return &dto.ServerResponse{
		ID:     1,
		UserID: userID,
		Name:   "Test Server",
		Status: "PENDING",
	}, nil
}

func (m *mockServerService) UpdateServer(ctx context.Context, id int64, userID int64, req dto.UpdateServerRequest) (*dto.ServerResponse, error) {
	if id != 1 {
		return nil, service.ErrServerNotFound
	}
	return &dto.ServerResponse{
		ID:     1,
		UserID: userID,
		Name:   req.Name,
		Status: "PENDING",
	}, nil
}

func (m *mockServerService) DeleteServer(ctx context.Context, id int64, userID int64) error {
	if id != 1 {
		return service.ErrServerNotFound
	}
	return nil
}

var _ service.ServerService = (*mockServerService)(nil)

// mockAuthService implements the ValidateToken portion needed for middleware testing
type mockAuthService struct {
	authService.AuthService
}

func (m *mockAuthService) ValidateToken(token string) (*authService.UserClaims, error) {
	if token == "valid-user-token" {
		return &authService.UserClaims{
			UserID: 42,
			Email:  "user@example.com",
			Role:   "USER",
			Name:   "Regular User",
		}, nil
	}
	return nil, authService.ErrInvalidToken
}

func TestServerMiddleware_NoJWT_Returns401(t *testing.T) {
	mockAuth := &mockAuthService{}
	mw := middleware.JWTRoleMiddleware(mockAuth)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	handler := mw(nextHandler)

	// 1. Request to /api/v1/user/servers without Authorization header
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/servers", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 Unauthorized, got %d", rr.Code)
	}

	// 2. Request with invalid token format
	reqInvalid := httptest.NewRequest(http.MethodGet, "/api/v1/user/servers", nil)
	reqInvalid.Header.Set("Authorization", "InvalidHeaderValue")
	rrInvalid := httptest.NewRecorder()

	handler.ServeHTTP(rrInvalid, reqInvalid)

	if rrInvalid.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401 for bad format, got %d", rrInvalid.Code)
	}

	// 3. Request with valid token
	reqValid := httptest.NewRequest(http.MethodGet, "/api/v1/user/servers", nil)
	reqValid.Header.Set("Authorization", "Bearer valid-user-token")
	rrValid := httptest.NewRecorder()

	handler.ServeHTTP(rrValid, reqValid)

	if rrValid.Code != http.StatusOK {
		t.Errorf("expected status 200 OK for valid token, got %d", rrValid.Code)
	}
}
