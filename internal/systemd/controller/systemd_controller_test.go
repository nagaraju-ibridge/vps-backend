package controller_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	agentMw "vpsmonitoring-backend/internal/agent/middleware"
	agentModels "vpsmonitoring-backend/internal/agent/models"
	agentService "vpsmonitoring-backend/internal/agent/service"
	"vpsmonitoring-backend/internal/systemd/cache"
	"vpsmonitoring-backend/internal/systemd/dto"
	systemdService "vpsmonitoring-backend/internal/systemd/service"
)

type mockAgentService struct {
	agentService.AgentService
}

func (m *mockAgentService) AuthenticateAgent(_ context.Context, rawCredential string) (*agentModels.Agent, error) {
	if rawCredential == "valid-agent-secret" {
		return &agentModels.Agent{AgentID: "agent-test", ServerID: 42, Status: "ACTIVE"}, nil
	}
	return nil, agentService.ErrUnauthorizedAgent
}

func buildPipeline(c cache.SystemdCache) http.Handler {
	svc := systemdService.NewSystemdService(c, nil)
	return agentMw.AgentAuthMiddleware(&mockAgentService{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		val := r.Context().Value(agentMw.AgentIdentityContextKey)
		if val == nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		identity := val.(*agentMw.AgentIdentity)

		var req dto.ServicePayloadDTO
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if req.CollectedAt.IsZero() || len(req.Services) > 200 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		err := svc.IngestSnapshot(identity.ServerID, req)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]int{"received": len(req.Services)})
	}))
}

func post(handler http.Handler, body any, credential string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/x/services", &buf)
	req.Header.Set("Content-Type", "application/json")
	if credential != "" {
		req.Header.Set("Authorization", "Bearer "+credential)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func TestIngestSnapshot_ValidPayload(t *testing.T) {
	c := cache.NewSystemdCache()
	h := buildPipeline(c)

	payload := dto.ServicePayloadDTO{
		CollectedAt: time.Now(),
		Services: []dto.ServiceSnapshotDTO{
			{Name: "nginx.service", ActiveState: "active", SubState: "running", LoadState: "loaded", CollectedAt: time.Now()},
		},
	}
	rr := post(h, payload, "valid-agent-secret")

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr.Code)
	}

	entry, ok := c.Get(42)
	if !ok || len(entry.Services) != 1 {
		t.Fatalf("expected cache update")
	}
}

func TestIngestSnapshot_InvalidAgent(t *testing.T) {
	c := cache.NewSystemdCache()
	h := buildPipeline(c)

	rr := post(h, dto.ServicePayloadDTO{CollectedAt: time.Now(), Services: []dto.ServiceSnapshotDTO{}}, "invalid-secret")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestIngestSnapshot_Validation(t *testing.T) {
	c := cache.NewSystemdCache()
	h := buildPipeline(c)

	// Missing collected_at
	rr := post(h, dto.ServicePayloadDTO{Services: []dto.ServiceSnapshotDTO{}}, "valid-agent-secret")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}

	// Too many services
	services := make([]dto.ServiceSnapshotDTO, 201)
	rr = post(h, dto.ServicePayloadDTO{CollectedAt: time.Now(), Services: services}, "valid-agent-secret")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for >200 services, got %d", rr.Code)
	}
}

func TestIngestSnapshot_EmptyList(t *testing.T) {
	c := cache.NewSystemdCache()
	h := buildPipeline(c)

	payload := dto.ServicePayloadDTO{
		CollectedAt: time.Now(),
		Services:    []dto.ServiceSnapshotDTO{},
	}
	rr := post(h, payload, "valid-agent-secret")
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr.Code)
	}
}
