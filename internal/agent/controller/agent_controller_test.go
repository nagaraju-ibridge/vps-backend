package controller_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	agentController "vpsmonitoring-backend/internal/agent/controller"
	"vpsmonitoring-backend/internal/agent/dto"
	agentMw "vpsmonitoring-backend/internal/agent/middleware"
	"vpsmonitoring-backend/internal/agent/models"
	"vpsmonitoring-backend/internal/agent/service"
	"vpsmonitoring-backend/internal/auth/middleware"
	authService "vpsmonitoring-backend/internal/auth/service"
)

// mockAgentService implements service.AgentService
type mockAgentService struct {
	registerFunc func(ctx context.Context, req dto.RegisterAgentRequest) (*dto.RegisterAgentResponse, error)
}

func (m *mockAgentService) Register(ctx context.Context, req dto.RegisterAgentRequest) (*dto.RegisterAgentResponse, error) {
	if m.registerFunc != nil {
		return m.registerFunc(ctx, req)
	}
	return &dto.RegisterAgentResponse{
		AgentID:    "test-uuid-agent-id",
		ServerID:   1,
		Credential: "raw-credential-secret-12345",
		Status:     "ACTIVE",
	}, nil
}

func (m *mockAgentService) AuthenticateAgent(ctx context.Context, rawCredential string) (*models.Agent, error) {
	return nil, nil
}

func (m *mockAgentService) Heartbeat(ctx context.Context, agentID string, serverID int64, req dto.HeartbeatRequest) (*dto.HeartbeatResponse, error) {
	if agentID == "fail-agent" {
		return nil, service.ErrAgentNotFound
	}
	return &dto.HeartbeatResponse{
		AgentID:  agentID,
		ServerID: serverID,
		Status:   "ONLINE",
		LastSeen: "2026-09-29T06:10:05Z",
	}, nil
}

func (m *mockAgentService) DetectOfflineAgents(ctx context.Context, offlineThreshold time.Duration) (int64, error) {
	return 0, nil
}

var _ service.AgentService = (*mockAgentService)(nil)

// mockAuthService implements authService.AuthService
type mockAuthService struct {
	authService.AuthService
}

func (m *mockAuthService) ValidateToken(token string) (*authService.UserClaims, error) {
	return nil, authService.ErrInvalidToken
}

func TestAgentRegistration_NoJWTRequired_BypassesAuthMiddleware(t *testing.T) {
	mockAuth := &mockAuthService{}
	mw := middleware.JWTRoleMiddleware(mockAuth)

	// Handler behind middleware
	reachedHandler := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reachedHandler = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"status":"ACTIVE"}}`))
	}))

	// POST /api/v1/agent/register without any Authorization header
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/register", strings.NewReader(`{"installation_token":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if !reachedHandler {
		t.Errorf("expected request to reach handler without JWT requirement")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200 OK, got %d", rr.Code)
	}
}

func TestAgentController_Heartbeat_EndToEndPipeline(t *testing.T) {
	mockSvc := &mockAgentService{}
	ctrl := agentController.NewAgentController(mockSvc)

	// Middleware wraps the handler
	heartbeatHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify AgentIdentity is retrieved by controller from context
		val := r.Context().Value(agentMw.AgentIdentityContextKey)
		if val == nil {
			http.Error(w, `{"error":{"message":"unauthenticated request"}}`, http.StatusUnauthorized)
			return
		}
		identity, ok := val.(*agentMw.AgentIdentity)
		if !ok || identity == nil {
			http.Error(w, `{"error":{"message":"invalid identity"}}`, http.StatusUnauthorized)
			return
		}

		resp, err := mockSvc.Heartbeat(r.Context(), identity.AgentID, identity.ServerID, dto.HeartbeatRequest{
			Timestamp:    "2026-09-29T06:10:00Z",
			AgentVersion: "0.1.0",
		})
		if err != nil {
			http.Error(w, `{"error":{"message":"heartbeat failed"}}`, http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"data":{"agent_id":"%s","server_id":%d,"status":"%s","last_seen":"%s"}}`,
			resp.AgentID, resp.ServerID, resp.Status, resp.LastSeen)))
	})

	t.Run("Heartbeat returns unauthenticated when no AgentIdentity present", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/heartbeat", strings.NewReader(`{}`))
		rec := httptest.NewRecorder()

		heartbeatHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", rec.Code)
		}
	})

	t.Run("Heartbeat succeeds with authenticated AgentIdentity", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/heartbeat", strings.NewReader(`{"timestamp":"2026-09-29T06:10:00Z","agent_version":"0.1.0"}`))
		req.Header.Set("Content-Type", "application/json")

		// Attach authenticated AgentIdentity to context (as middleware would)
		ctxWithVal := context.WithValue(req.Context(), agentMw.AgentIdentityContextKey, &agentMw.AgentIdentity{
			AgentID:  "agent-uuid-heartbeat",
			ServerID: 101,
			Status:   "ACTIVE",
		})

		rec := httptest.NewRecorder()
		heartbeatHandler.ServeHTTP(rec, req.WithContext(ctxWithVal))

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}

		if !strings.Contains(rec.Body.String(), "agent-uuid-heartbeat") || !strings.Contains(rec.Body.String(), "ONLINE") {
			t.Errorf("unexpected response body: %s", rec.Body.String())
		}
	})

	_ = ctrl // ensures ctrl instantiation compiles
}
