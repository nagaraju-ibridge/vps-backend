package controller_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gofr.dev/pkg/gofr"

	authMw "vpsmonitoring-backend/internal/auth/middleware"
	authService "vpsmonitoring-backend/internal/auth/service"
	"vpsmonitoring-backend/internal/process/cache"
	"vpsmonitoring-backend/internal/process/controller"
	"vpsmonitoring-backend/internal/process/dto"
	processService "vpsmonitoring-backend/internal/process/service"
	serverDto "vpsmonitoring-backend/internal/server/dto"
	serverService "vpsmonitoring-backend/internal/server/service"
)

// fakeGofrRequest implements gofr.Request for testing
type fakeGofrRequest struct {
	ctx        context.Context
	pathParam  map[string]string
	queryParam map[string]string
}

func (r fakeGofrRequest) Param(s string) string     { return r.queryParam[s] }
func (r fakeGofrRequest) Params(s string) []string  { return []string{r.queryParam[s]} }
func (r fakeGofrRequest) PathParam(s string) string { return r.pathParam[s] }
func (r fakeGofrRequest) Context() context.Context  { return r.ctx }
func (r fakeGofrRequest) Bind(i interface{}) error  { return nil }
func (r fakeGofrRequest) HostName() string          { return "localhost" }

type mockServerService struct {
	serverService.ServerService
	servers map[int64]map[int64]bool // serverID -> userID -> true
}

func (m *mockServerService) GetServer(ctx context.Context, id int64, userID int64) (*serverDto.ServerResponse, error) {
	users, ok := m.servers[id]
	if !ok {
		return nil, serverService.ErrServerNotFound
	}
	if !users[userID] {
		return nil, serverService.ErrServerNotFound
	}
	return &serverDto.ServerResponse{ID: id, UserID: userID}, nil
}

func newCtx(serverID string, userID int64, query map[string]string) *gofr.Context {
	base := context.WithValue(context.Background(), authMw.UserClaimsContextKey, &authService.UserClaims{
		UserID: userID,
	})
	return &gofr.Context{
		Context: base,
		Request: fakeGofrRequest{
			ctx:        base,
			pathParam:  map[string]string{"serverId": serverID},
			queryParam: query,
		},
	}
}

func TestGetProcesses_Success(t *testing.T) {
	c := cache.NewProcessCache()
	now := time.Now().UTC()
	cpu20 := 20.0
	cpu10 := 10.0
	c.Set(10, now, []dto.ProcessSnapshotDTO{
		{PID: 2, Name: "worker", CPUPercent: &cpu10},
		{PID: 1, Name: "init", CPUPercent: &cpu20},
	})

	srvServ := &mockServerService{servers: map[int64]map[int64]bool{
		10: {5: true},
	}}

	svc := processService.NewProcessService(c, srvServ)
	ctrl := controller.NewProcessController(svc)

	ctx := newCtx("10", 5, map[string]string{"sort": "cpu_desc", "limit": "10"})
	res, err := ctrl.GetProcesses(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp, ok := res.(*dto.GetProcessSnapshotResponse)
	if !ok {
		t.Fatalf("expected *dto.GetProcessSnapshotResponse")
	}

	if len(resp.Processes) != 2 {
		t.Fatalf("expected 2 processes, got %d", len(resp.Processes))
	}
	if resp.Processes[0].PID != 1 {
		t.Errorf("expected PID 1 first due to cpu_desc, got PID %d", resp.Processes[0].PID)
	}
	if resp.Total != 2 || resp.Page != 1 || resp.PageSize != 10 || resp.TotalPages != 1 {
		t.Errorf("unexpected pagination metadata: total=%d page=%d page_size=%d total_pages=%d", resp.Total, resp.Page, resp.PageSize, resp.TotalPages)
	}
}

func TestGetProcesses_ServerSidePagination(t *testing.T) {
	c := cache.NewProcessCache()
	processes := make([]dto.ProcessSnapshotDTO, 60)
	for i := range processes {
		cpu := float64(i)
		processes[i] = dto.ProcessSnapshotDTO{PID: int64(i + 1), Name: "proc", CPUPercent: &cpu}
	}
	c.Set(10, time.Now().UTC(), processes)
	srvServ := &mockServerService{servers: map[int64]map[int64]bool{10: {5: true}}}
	ctrl := controller.NewProcessController(processService.NewProcessService(c, srvServ))

	res, err := ctrl.GetProcesses(newCtx("10", 5, map[string]string{"page": "2", "limit": "25", "sort": "cpu_desc"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp := res.(*dto.GetProcessSnapshotResponse)
	if len(resp.Processes) != 25 || resp.Processes[0].PID != 35 || resp.Processes[24].PID != 11 {
		t.Fatalf("unexpected second page after server sort: first=%d last=%d count=%d", resp.Processes[0].PID, resp.Processes[24].PID, len(resp.Processes))
	}
	if resp.Total != 60 || resp.Page != 2 || resp.PageSize != 25 || resp.TotalPages != 3 {
		t.Fatalf("unexpected pagination metadata: %+v", resp)
	}
}

func TestGetProcesses_CrossTenant(t *testing.T) {
	c := cache.NewProcessCache()
	srvServ := &mockServerService{servers: map[int64]map[int64]bool{
		10: {5: true}, // Belongs to user 5
	}}

	svc := processService.NewProcessService(c, srvServ)
	ctrl := controller.NewProcessController(svc)

	ctx := newCtx("10", 99, nil)
	_, err := ctrl.GetProcesses(ctx)

	if !errors.Is(err, serverService.ErrServerNotFound) {
		t.Fatalf("expected ErrServerNotFound, got %v", err)
	}
}

func TestGetProcesses_SnapshotMissing(t *testing.T) {
	c := cache.NewProcessCache()
	srvServ := &mockServerService{servers: map[int64]map[int64]bool{
		10: {5: true},
	}}

	svc := processService.NewProcessService(c, srvServ)
	ctrl := controller.NewProcessController(svc)

	ctx := newCtx("10", 5, nil)
	_, err := ctrl.GetProcesses(ctx)

	// Since we defined our own custom error matching "snapshot_missing" we can check string
	if err == nil || err.Error() != "snapshot_missing" {
		t.Fatalf("expected snapshot_missing error, got %v", err)
	}

	// Verify status code method
	type sc interface {
		StatusCode() int
	}
	scErr, ok := err.(sc)
	if !ok {
		t.Fatalf("error should implement StatusCode()")
	}
	if scErr.StatusCode() != 503 {
		t.Fatalf("expected 503, got %d", scErr.StatusCode())
	}
}

// ----------------------------------------------------------------------------
// PHASE 3.4.12: SECURITY & VALIDATION TESTS
// ----------------------------------------------------------------------------

func TestGetProcesses_Authentication(t *testing.T) {
	// 1. AUTHENTICATION (Missing, Invalid, Expired JWT -> 401)
	// We verify the controller rejects unauthenticated contexts natively.
	// (The actual JWT parsing is handled in auth_middleware_test.go, but we ensure
	// the controller itself enforces the presence of claims).
	c := cache.NewProcessCache()
	srvServ := &mockServerService{servers: map[int64]map[int64]bool{10: {5: true}}}
	svc := processService.NewProcessService(c, srvServ)
	ctrl := controller.NewProcessController(svc)

	// Missing claims entirely
	ctx1 := &gofr.Context{Context: context.Background(), Request: fakeGofrRequest{ctx: context.Background()}}
	_, err := ctrl.GetProcesses(ctx1)
	if err == nil || err.Error() != "unauthenticated request" {
		t.Errorf("expected 'unauthenticated request', got %v", err)
	}
}

func TestGetProcesses_ServerValidation(t *testing.T) {
	// 4. SERVER VALIDATION (Invalid server ID, Nonexistent server, Wrong user)
	c := cache.NewProcessCache()
	srvServ := &mockServerService{servers: map[int64]map[int64]bool{10: {5: true}}}
	svc := processService.NewProcessService(c, srvServ)
	ctrl := controller.NewProcessController(svc)

	// Invalid server ID
	ctxInvalid := newCtx("not-a-number", 5, nil)
	_, err := ctrl.GetProcesses(ctxInvalid)
	if err == nil || err.Error() != "valid server ID is required" {
		t.Errorf("expected 'valid server ID is required', got %v", err)
	}

	// Nonexistent server
	ctxNotFound := newCtx("99", 5, nil)
	_, err = ctrl.GetProcesses(ctxNotFound)
	if !errors.Is(err, serverService.ErrServerNotFound) {
		t.Errorf("expected ErrServerNotFound, got %v", err)
	}
}

func TestGetProcesses_QueryValidation(t *testing.T) {
	// 5. QUERY VALIDATION (limit and sort)
	c := cache.NewProcessCache()
	c.Set(10, time.Now().UTC(), []dto.ProcessSnapshotDTO{{PID: 1, Name: "init"}})
	srvServ := &mockServerService{servers: map[int64]map[int64]bool{10: {5: true}}}
	svc := processService.NewProcessService(c, srvServ)
	ctrl := controller.NewProcessController(svc)

	tests := []struct {
		name    string
		limit   string
		sort    string
		wantErr error
	}{
		{"omitted limit -> success", "", "", nil},
		{"limit 1 -> success", "1", "", nil},
		{"limit 100 -> success", "100", "", nil},
		{"limit 0 -> 400", "0", "", processService.ErrInvalidLimit},
		{"limit negative -> 400", "-5", "", processService.ErrInvalidLimit},
		{"limit 101 -> 400", "101", "", processService.ErrInvalidLimit},
		{"limit malformed -> 400", "abc", "", processService.ErrInvalidLimit},

		{"sort omitted -> success", "", "", nil},
		{"sort cpu_desc -> success", "", "cpu_desc", nil},
		{"sort cpu_asc -> success", "", "cpu_asc", nil},
		{"sort memory_desc -> success", "", "memory_desc", nil},
		{"sort memory_asc -> success", "", "memory_asc", nil},
		{"sort pid -> success", "", "pid", nil},
		{"sort invalid -> 400", "", "invalid_sort", processService.ErrInvalidSort},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := newCtx("10", 5, map[string]string{"limit": tt.limit, "sort": tt.sort})
			_, err := ctrl.GetProcesses(ctx)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("expected error %v, got %v", tt.wantErr, err)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestGetProcesses_SortAndLimitSecurity(t *testing.T) {
	// 8. SORTING AND LIMIT SECURITY (Sort occurs before limit)
	c := cache.NewProcessCache()

	// Create 60 processes
	processes := make([]dto.ProcessSnapshotDTO, 60)
	for i := 0; i < 60; i++ {
		cpu := float64(i) // 0 to 59
		processes[i] = dto.ProcessSnapshotDTO{
			PID:        int64(i + 1),
			Name:       "proc",
			CPUPercent: &cpu,
		}
	}
	c.Set(10, time.Now().UTC(), processes)
	srvServ := &mockServerService{servers: map[int64]map[int64]bool{10: {5: true}}}
	svc := processService.NewProcessService(c, srvServ)
	ctrl := controller.NewProcessController(svc)

	// limit=50, sort=cpu_desc -> should return PIDs 60 down to 11
	ctx := newCtx("10", 5, map[string]string{"limit": "50", "sort": "cpu_desc"})
	res, err := ctrl.GetProcesses(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := res.(*dto.GetProcessSnapshotResponse)
	if len(resp.Processes) != 50 {
		t.Fatalf("expected 50 processes, got %d", len(resp.Processes))
	}

	// Highest CPU is i=59, PID=60.
	if resp.Processes[0].PID != 60 {
		t.Errorf("expected PID 60 to be first (highest CPU), got %d", resp.Processes[0].PID)
	}
}

func TestGetProcesses_EmptySnapshot(t *testing.T) {
	// 7. SNAPSHOT STATES - Existing empty snapshot -> 200 with processes:[]
	c := cache.NewProcessCache()
	c.Set(10, time.Now().UTC(), []dto.ProcessSnapshotDTO{})
	srvServ := &mockServerService{servers: map[int64]map[int64]bool{10: {5: true}}}
	svc := processService.NewProcessService(c, srvServ)
	ctrl := controller.NewProcessController(svc)

	ctx := newCtx("10", 5, nil)
	res, err := ctrl.GetProcesses(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resp := res.(*dto.GetProcessSnapshotResponse)
	if len(resp.Processes) != 0 {
		t.Errorf("expected empty processes, got %d", len(resp.Processes))
	}
}

func TestGetProcesses_ResponseSecurity(t *testing.T) {
	// 6. RESPONSE SECURITY
	// Verify serialized JSON does NOT contain sensitive fields and optional fields
	// are null.

	c := cache.NewProcessCache()
	c.Set(10, time.Now().UTC(), []dto.ProcessSnapshotDTO{
		{
			PID:  1234,
			Name: "my-proc",
			// Leave optional fields nil
			CPUPercent:    nil,
			MemoryPercent: nil,
		},
	})
	srvServ := &mockServerService{servers: map[int64]map[int64]bool{10: {5: true}}}
	svc := processService.NewProcessService(c, srvServ)
	ctrl := controller.NewProcessController(svc)

	ctx := newCtx("10", 5, nil)
	res, _ := ctrl.GetProcesses(ctx)

	b, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	jsonStr := string(b)

	// Check for omitted sensitive terms
	sensitive := []string{"cmdline", "environ", "environment", "open_files", "connections", "sockets", "password", "token", "secret", "jwt", "authorization", "credentials"}
	for _, term := range sensitive {
		if strings.Contains(strings.ToLower(jsonStr), term) {
			t.Errorf("JSON response contains sensitive term %q: %s", term, jsonStr)
		}
	}

	// Since the DTO uses `omitempty`, optional nil fields will simply be omitted
	// from the output entirely, which safely fulfills the DTO requirements.
	if strings.Contains(jsonStr, `"cpu_percent"`) {
		t.Errorf("expected cpu_percent to be omitted, got %s", jsonStr)
	}
	if strings.Contains(jsonStr, `"memory_percent"`) {
		t.Errorf("expected memory_percent to be omitted, got %s", jsonStr)
	}
}

type mockAuthService struct {
	authService.AuthService
}

func (m *mockAuthService) ValidateToken(token string) (*authService.UserClaims, error) {
	if token == "valid-user-token" {
		return &authService.UserClaims{UserID: 5, Role: "USER"}, nil
	}
	if token == "expired-user-token" {
		return nil, errors.New("expired token")
	}
	// Agent credential/Installation token should either be rejected or map to a non-user claims.
	// We simulate them failing Validation.
	if token == "agent-token" || token == "installation-token" {
		return nil, errors.New("invalid token format for user auth")
	}
	return nil, errors.New("invalid token")
}

func TestGetProcesses_PipelineSecurity(t *testing.T) {
	// Tests:
	// Missing JWT -> 401
	// Invalid JWT -> 401
	// Expired JWT -> 401
	// Valid user JWT -> request proceeds (which we mock to 404 since we don't start the full app)
	// Agent JWT/credential -> 401
	// Installation token -> 401

	authMock := &mockAuthService{}
	mw := authMw.JWTRoleMiddleware(authMock)

	// A dummy handler that represents the protected resource
	protectedHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	})

	pipeline := mw(protectedHandler)

	tests := []struct {
		name       string
		authHeader string
		wantCode   int
	}{
		{"Missing JWT -> 401", "", http.StatusUnauthorized},
		{"Invalid format -> 401", "InvalidTokenFormat", http.StatusUnauthorized},
		{"Invalid JWT -> 401", "Bearer invalid-token", http.StatusUnauthorized},
		{"Expired JWT -> 401", "Bearer expired-user-token", http.StatusUnauthorized},
		{"Valid user JWT -> proceeds (200)", "Bearer valid-user-token", http.StatusOK},
		{"Agent JWT -> 401", "Bearer agent-token", http.StatusUnauthorized},
		{"Installation token -> 401", "Bearer installation-token", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/user/servers/10/processes", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			w := httptest.NewRecorder()
			pipeline.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("expected status %d, got %d. Body: %s", tt.wantCode, w.Code, w.Body.String())
			}
		})
	}
}
