package middleware_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gofr.dev/pkg/gofr"

	agentMiddleware "vpsmonitoring-backend/internal/agent/middleware"
	agentModels "vpsmonitoring-backend/internal/agent/models"
	agentService "vpsmonitoring-backend/internal/agent/service"
	authMiddleware "vpsmonitoring-backend/internal/auth/middleware"
	authModels "vpsmonitoring-backend/internal/auth/models"
	authService "vpsmonitoring-backend/internal/auth/service"
)

// mockAgentServiceAuth implements agentService.AgentService for testing middleware
type mockAgentServiceAuth struct {
	agentService.AgentService
	validCredential string
	agent           *agentModels.Agent
	err             error
}

func (m *mockAgentServiceAuth) AuthenticateAgent(ctx context.Context, rawCredential string) (*agentModels.Agent, error) {
	if m.err != nil {
		return nil, m.err
	}
	if rawCredential == m.validCredential && m.agent != nil {
		return m.agent, nil
	}
	return nil, agentService.ErrUnauthorizedAgent
}

// mockAuthService implements authService.AuthService for cross-testing user JWT isolation
type mockAuthService struct {
	authService.AuthService
	validToken string
	claims     *authService.UserClaims
}

func (m *mockAuthService) ValidateToken(token string) (*authService.UserClaims, error) {
	if token == m.validToken && m.claims != nil {
		return m.claims, nil
	}
	return nil, authService.ErrInvalidToken
}

func TestAgentAuthMiddleware(t *testing.T) {
	validCred := "agent-secret-credential-abcdef-123456"
	agent := &agentModels.Agent{
		ID:       42,
		ServerID: 99,
		AgentID:  "agent-uuid-42",
		Status:   agentModels.AgentStatusActive,
		Version:  "0.1.0",
	}

	mockSvc := &mockAgentServiceAuth{
		validCredential: validCred,
		agent:           agent,
	}

	middleware := agentMiddleware.AgentAuthMiddleware(mockSvc)

	// Sample downstream handler
	var capturedIdentity *agentMiddleware.AgentIdentity
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		val := r.Context().Value(agentMiddleware.AgentIdentityContextKey)
		if val != nil {
			if id, ok := val.(*agentMiddleware.AgentIdentity); ok {
				capturedIdentity = id
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	handlerToTest := middleware(nextHandler)

	t.Run("Valid agent credential succeeds and propagates identity", func(t *testing.T) {
		capturedIdentity = nil
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/heartbeat", nil)
		req.Header.Set("Authorization", "Bearer "+validCred)
		rec := httptest.NewRecorder()

		handlerToTest.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
		}
		if capturedIdentity == nil {
			t.Fatal("expected captured identity in context, got nil")
		}
		if capturedIdentity.AgentID != "agent-uuid-42" {
			t.Errorf("expected agent ID agent-uuid-42, got %s", capturedIdentity.AgentID)
		}
		if capturedIdentity.ServerID != 99 {
			t.Errorf("expected server ID 99, got %d", capturedIdentity.ServerID)
		}
	})

	t.Run("Missing Authorization header returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/heartbeat", nil)
		rec := httptest.NewRecorder()

		handlerToTest.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
		var errResp map[string]map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
		if errResp["error"]["message"] == "" {
			t.Errorf("expected error message in response")
		}
	})

	t.Run("Malformed Authorization header (no Bearer) returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/heartbeat", nil)
		req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
		rec := httptest.NewRecorder()

		handlerToTest.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("Empty Bearer credential returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/heartbeat", nil)
		req.Header.Set("Authorization", "Bearer    ")
		rec := httptest.NewRecorder()

		handlerToTest.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("Invalid/wrong credential returns 401 without detail leaks", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/heartbeat", nil)
		req.Header.Set("Authorization", "Bearer invalid-or-wrong-cred")
		rec := httptest.NewRecorder()

		handlerToTest.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
		var errResp map[string]map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &errResp)
		if errResp["error"]["message"] != "Unauthorized: invalid agent credential" {
			t.Errorf("expected generic error message, got %s", errResp["error"]["message"])
		}
	})

	t.Run("Public agent registration route /api/v1/agent/register is bypassed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/register", nil)
		rec := httptest.NewRecorder()

		handlerToTest.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 (bypassed), got %d", rec.Code)
		}
	})

	t.Run("Non-agent routes are bypassed by agent middleware", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/user/servers", nil)
		rec := httptest.NewRecorder()

		handlerToTest.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 (bypassed), got %d", rec.Code)
		}
	})
}

func TestAgentIdentityHelper(t *testing.T) {
	t.Run("GetAgentIdentity returns identity when present in context", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		expectedID := &agentMiddleware.AgentIdentity{
			AgentID:  "agent-test-1",
			ServerID: 10,
			Status:   "ACTIVE",
		}
		ctxWithVal := context.WithValue(req.Context(), agentMiddleware.AgentIdentityContextKey, expectedID)
		gofrCtx := &gofr.Context{Context: ctxWithVal}

		identity, err := agentMiddleware.GetAgentIdentity(gofrCtx)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if identity.AgentID != "agent-test-1" || identity.ServerID != 10 {
			t.Errorf("unexpected identity: %+v", identity)
		}
	})

	t.Run("GetAgentIdentity returns error when context is nil or value missing", func(t *testing.T) {
		_, err := agentMiddleware.GetAgentIdentity(nil)
		if err == nil {
			t.Error("expected error for nil context")
		}

		gofrCtx := &gofr.Context{Context: context.Background()}
		_, err = agentMiddleware.GetAgentIdentity(gofrCtx)
		if err == nil {
			t.Error("expected error when identity not in context")
		}
	})
}

func TestAgentAndUserAuthSeparation(t *testing.T) {
	// Setup Agent Middleware
	validAgentCred := "agent-valid-cred"
	mockAgentSvc := &mockAgentServiceAuth{
		validCredential: validAgentCred,
		agent: &agentModels.Agent{
			AgentID:  "agent-x",
			ServerID: 55,
			Status:   agentModels.AgentStatusActive,
		},
	}
	agentMw := agentMiddleware.AgentAuthMiddleware(mockAgentSvc)

	// Setup User JWT Middleware
	validUserJWT := "valid-user-jwt-token"
	mockUserSvc := &mockAuthService{
		validToken: validUserJWT,
		claims: &authService.UserClaims{
			UserID: 1,
			Email:  "user@example.com",
			Role:   authModels.RoleUser,
		},
	}
	userMw := authMiddleware.JWTRoleMiddleware(mockUserSvc)

	echoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success":true}`))
	})

	t.Run("User JWT cannot authenticate Agent endpoint", func(t *testing.T) {
		agentHandler := agentMw(echoHandler)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/heartbeat", nil)
		// Send valid User JWT to agent endpoint
		req.Header.Set("Authorization", "Bearer "+validUserJWT)
		rec := httptest.NewRecorder()

		agentHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401 when sending User JWT to agent endpoint, got %d", rec.Code)
		}
	})

	t.Run("Agent credential cannot authenticate User endpoint", func(t *testing.T) {
		userHandler := userMw(echoHandler)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/user/servers", nil)
		// Send valid Agent credential to user endpoint
		req.Header.Set("Authorization", "Bearer "+validAgentCred)
		rec := httptest.NewRecorder()

		userHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401 when sending Agent credential to user endpoint, got %d", rec.Code)
		}
	})
}
