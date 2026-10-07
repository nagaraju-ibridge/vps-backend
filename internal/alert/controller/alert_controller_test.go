package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"gofr.dev/pkg/gofr"
	"vpsmonitoring-backend/internal/alert/dto"
	alertService "vpsmonitoring-backend/internal/alert/service"
	authMiddleware "vpsmonitoring-backend/internal/auth/middleware"
	authService "vpsmonitoring-backend/internal/auth/service"
)

type alertRequest struct {
	ctx         context.Context
	path, query map[string]string
}

func (r alertRequest) Param(key string) string     { return r.query[key] }
func (r alertRequest) Params(key string) []string  { return []string{r.query[key]} }
func (r alertRequest) PathParam(key string) string { return r.path[key] }
func (r alertRequest) Context() context.Context    { return r.ctx }
func (r alertRequest) Bind(any) error              { return nil }
func (r alertRequest) HostName() string            { return "localhost" }
func alertContext(userID int64, path, query map[string]string) *gofr.Context {
	base := context.Background()
	if userID > 0 {
		base = context.WithValue(base, authMiddleware.UserClaimsContextKey, &authService.UserClaims{UserID: userID})
	}
	return &gofr.Context{Context: base, Request: alertRequest{ctx: base, path: path, query: query}}
}

type alertQueryServiceMock struct {
	alertService.AlertQueryService
	filter                  dto.AlertListFilter
	serverID, applicationID int64
	err                     error
}

func (m *alertQueryServiceMock) List(_ context.Context, _ int64, filter dto.AlertListFilter) (*dto.AlertListResponse, error) {
	m.filter = filter
	return &dto.AlertListResponse{Alerts: []dto.AlertResponse{}, Page: filter.Page, Limit: filter.Limit}, m.err
}
func (m *alertQueryServiceMock) Get(context.Context, int64, uuid.UUID) (*dto.AlertResponse, error) {
	return &dto.AlertResponse{}, m.err
}
func (m *alertQueryServiceMock) ListByServer(_ context.Context, _ int64, serverID int64, filter dto.AlertListFilter) (*dto.AlertListResponse, error) {
	m.serverID = serverID
	m.filter = filter
	return &dto.AlertListResponse{}, m.err
}
func (m *alertQueryServiceMock) ListByApplication(_ context.Context, _ int64, serverID, applicationID int64, filter dto.AlertListFilter) (*dto.AlertListResponse, error) {
	m.serverID = serverID
	m.applicationID = applicationID
	m.filter = filter
	return &dto.AlertListResponse{}, m.err
}

func statusCode(err error) int {
	type statusCoder interface{ StatusCode() int }
	if typed, ok := err.(statusCoder); ok {
		return typed.StatusCode()
	}
	return 0
}
func TestAlertControllerParsingAndErrors(t *testing.T) {
	mock := &alertQueryServiceMock{}
	controller := NewAlertController(mock)
	result, err := controller.List(alertContext(10, nil, map[string]string{"page": "2", "limit": "25", "status": "ACTIVE", "server_id": "1", "application_id": "7"}))
	if err != nil || result == nil || mock.filter.Page != 2 || mock.filter.Limit != 25 || mock.filter.ServerID == nil || mock.filter.ApplicationID == nil {
		t.Fatalf("list parsing failed: %+v %v", mock.filter, err)
	}
	_, err = controller.Get(alertContext(10, map[string]string{"id": "invalid"}, nil))
	if statusCode(err) != http.StatusBadRequest {
		t.Fatalf("invalid uuid status=%d err=%v", statusCode(err), err)
	}
	_, err = controller.List(alertContext(0, nil, nil))
	if statusCode(err) != http.StatusUnauthorized {
		t.Fatalf("missing auth status=%d", statusCode(err))
	}
	mock.err = alertService.ErrInvalidAlertStatus
	_, err = controller.List(alertContext(10, nil, nil))
	if statusCode(err) != http.StatusBadRequest {
		t.Fatalf("validation status=%d", statusCode(err))
	}
	mock.err = alertService.ErrAlertResourceNotFound
	_, err = controller.ListByServer(alertContext(10, map[string]string{"serverId": "2"}, nil))
	if statusCode(err) != http.StatusNotFound {
		t.Fatalf("ownership status=%d", statusCode(err))
	}
}
func TestAlertControllerPathScopes(t *testing.T) {
	mock := &alertQueryServiceMock{}
	controller := NewAlertController(mock)
	if _, err := controller.ListByApplication(alertContext(10, map[string]string{"serverId": "1", "applicationId": "7"}, map[string]string{"limit": "10"})); err != nil {
		t.Fatal(err)
	}
	if mock.serverID != 1 || mock.applicationID != 7 || mock.filter.Limit != 10 {
		t.Fatalf("path scope not forwarded: %+v", mock)
	}
	if _, err := controller.ListByServer(alertContext(10, map[string]string{"serverId": "bad"}, nil)); statusCode(err) != http.StatusBadRequest {
		t.Fatalf("invalid server path: %v", err)
	}
}

type rejectingAuthService struct{ authService.AuthService }

func (*rejectingAuthService) ValidateToken(string) (*authService.UserClaims, error) {
	return nil, authService.ErrInvalidToken
}
func TestAlertUserRouteRejectsAgentCredential(t *testing.T) {
	called := false
	handler := authMiddleware.JWTRoleMiddleware(&rejectingAuthService{})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/user/alerts", nil)
	request.Header.Set("Authorization", "Bearer agent-credential")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized || called {
		t.Fatalf("agent credential was not rejected: status=%d called=%v", recorder.Code, called)
	}
}
