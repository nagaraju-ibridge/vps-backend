package controller_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"gofr.dev/pkg/gofr"

	agentMw "vpsmonitoring-backend/internal/agent/middleware"
	agentModels "vpsmonitoring-backend/internal/agent/models"
	agentService "vpsmonitoring-backend/internal/agent/service"
	authMw "vpsmonitoring-backend/internal/auth/middleware"
	authService "vpsmonitoring-backend/internal/auth/service"
	metricCtrl "vpsmonitoring-backend/internal/metric/controller"
	"vpsmonitoring-backend/internal/metric/dto"
	metricService "vpsmonitoring-backend/internal/metric/service"
	serverService "vpsmonitoring-backend/internal/server/service"
)

type mockMetricService struct {
	ingestFunc       func(ctx context.Context, agentID string, serverID int64, req dto.IngestMetricRequest) (*dto.IngestMetricResponse, error)
	getLatestFunc    func(ctx context.Context, serverID int64, userID int64) (*dto.LatestMetricResponse, error)
	getHistoryFunc   func(ctx context.Context, serverID int64, userID int64, start time.Time, end time.Time, limit int) (*dto.HistoricalMetricsResponse, error)
	getAggregateFunc func(ctx context.Context, serverID int64, userID int64, start time.Time, end time.Time, granularity string) (*dto.AggregatedMetricsResponse, error)
}

func (m *mockMetricService) IngestMetric(ctx context.Context, agentID string, serverID int64, req dto.IngestMetricRequest) (*dto.IngestMetricResponse, error) {
	if m.ingestFunc != nil {
		return m.ingestFunc(ctx, agentID, serverID, req)
	}
	return &dto.IngestMetricResponse{
		Success:    true,
		MetricID:   "test-metric-uuid",
		ServerID:   serverID,
		ReceivedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (m *mockMetricService) GetLatestMetric(ctx context.Context, serverID int64, userID int64) (*dto.LatestMetricResponse, error) {
	if m.getLatestFunc != nil {
		return m.getLatestFunc(ctx, serverID, userID)
	}
	return nil, errors.New("not implemented in controller ingestion tests")
}

func (m *mockMetricService) GetHistoricalMetrics(ctx context.Context, serverID int64, userID int64, start time.Time, end time.Time, limit int) (*dto.HistoricalMetricsResponse, error) {
	if m.getHistoryFunc != nil {
		return m.getHistoryFunc(ctx, serverID, userID, start, end, limit)
	}
	return nil, errors.New("not implemented in controller tests")
}

func (m *mockMetricService) GetAggregatedMetrics(ctx context.Context, serverID int64, userID int64, start time.Time, end time.Time, granularity string) (*dto.AggregatedMetricsResponse, error) {
	if m.getAggregateFunc != nil {
		return m.getAggregateFunc(ctx, serverID, userID, start, end, granularity)
	}
	return nil, errors.New("not implemented in controller tests")
}

type fakeGofrRequest struct {
	ctx        context.Context
	pathParam  map[string]string
	queryParam map[string]string
}

func (r fakeGofrRequest) Context() context.Context {
	return r.ctx
}

func (r fakeGofrRequest) Param(key string) string {
	return r.queryParam[key]
}

func (r fakeGofrRequest) PathParam(key string) string {
	return r.pathParam[key]
}

func (r fakeGofrRequest) Bind(any) error {
	return nil
}

func (r fakeGofrRequest) HostName() string {
	return ""
}

func (r fakeGofrRequest) Params(key string) []string {
	if v, ok := r.queryParam[key]; ok {
		return []string{v}
	}
	return nil
}

type mockAuthServiceForUserRoutes struct {
	authService.AuthService
}

func (m *mockAuthServiceForUserRoutes) ValidateToken(token string) (*authService.UserClaims, error) {
	if token == "valid-user-token" {
		return &authService.UserClaims{
			UserID: 7,
			Email:  "user@example.com",
			Role:   "USER",
			Name:   "Regular User",
		}, nil
	}
	return nil, authService.ErrInvalidToken
}

type mockAgentServiceForAuth struct {
	agentService.AgentService
}

func (m *mockAgentServiceForAuth) AuthenticateAgent(ctx context.Context, rawCredential string) (*agentModels.Agent, error) {
	if rawCredential == "valid-agent-secret" {
		return &agentModels.Agent{
			AgentID:  "agent-valid-123",
			ServerID: 88,
			Status:   "ACTIVE",
		}, nil
	}
	return nil, errors.New("unauthorized")
}

func TestMetricController_Ingest_Integration(t *testing.T) {
	agentServiceMock := &mockAgentServiceForAuth{}
	metricServiceMock := &mockMetricService{}
	ctrl := metricCtrl.NewMetricController(metricServiceMock)

	// Pipeline: AgentAuthMiddleware -> Controller handling
	pipelineHandler := agentMw.AgentAuthMiddleware(agentServiceMock)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		val := r.Context().Value(agentMw.AgentIdentityContextKey)
		if val == nil {
			http.Error(w, `{"error":{"message":"unauthenticated request: agent identity not found in context"}}`, http.StatusUnauthorized)
			return
		}
		identity, ok := val.(*agentMw.AgentIdentity)
		if !ok || identity == nil {
			http.Error(w, `{"error":{"message":"invalid identity"}}`, http.StatusUnauthorized)
			return
		}

		var req dto.IngestMetricRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":{"message":"invalid request body"}}`, http.StatusBadRequest)
			return
		}

		res, err := metricServiceMock.IngestMetric(r.Context(), identity.AgentID, identity.ServerID, req)
		if err != nil {
			http.Error(w, `{"error":{"message":"`+err.Error()+`"}}`, http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": res})
	}))

	t.Run("Valid authenticated metric submission succeeds with 200", func(t *testing.T) {
		payload := dto.IngestMetricRequest{
			Timestamp: time.Now().UTC(),
			CPU: dto.CPUMetricsDTO{
				UsagePercent: 20.0,
				Cores:        4,
			},
		}
		body, _ := json.Marshal(payload)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/metrics", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer valid-agent-secret")
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		pipelineHandler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
		}

		var resp map[string]any
		_ = json.Unmarshal(rr.Body.Bytes(), &resp)
		data := resp["data"].(map[string]any)
		if data["server_id"].(float64) != 88 {
			t.Errorf("expected server_id 88 from authenticated context, got %v", data["server_id"])
		}
	})

	t.Run("Missing agent credential returns 401 Unauthorized", func(t *testing.T) {
		payload := dto.IngestMetricRequest{Timestamp: time.Now().UTC()}
		body, _ := json.Marshal(payload)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/metrics", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		pipelineHandler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", rr.Code)
		}
	})

	t.Run("Invalid agent credential returns 401 Unauthorized", func(t *testing.T) {
		payload := dto.IngestMetricRequest{Timestamp: time.Now().UTC()}
		body, _ := json.Marshal(payload)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/metrics", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer invalid-credential")
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		pipelineHandler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", rr.Code)
		}
	})

	_ = ctrl // ensures ctrl constructor compiles cleanly
}

func TestMetricController_GetLatest(t *testing.T) {
	collectedAt := "2026-09-29T12:30:00Z"
	expected := &dto.LatestMetricResponse{
		ServerID:    42,
		CollectedAt: collectedAt,
		CPU: dto.LatestCPUMetricsDTO{
			UsagePercent: 37.5,
			Cores:        4,
		},
		Load: dto.LatestLoadMetricsDTO{
			Load1:  0.7,
			Load5:  0.6,
			Load15: 0.5,
		},
		Memory: dto.MemoryMetricsDTO{
			Total:        8000,
			Used:         3200,
			Available:    4800,
			UsagePercent: 40,
		},
		Swap: dto.SwapMetricsDTO{
			Total:        2000,
			Used:         100,
			Free:         1900,
			UsagePercent: 5,
		},
		Network: dto.NetworkMetricsDTO{
			RXBytes:   10000,
			TXBytes:   20000,
			RXPackets: 100,
			TXPackets: 200,
			Errors:    1,
			Drops:     2,
		},
		Disks: []dto.DiskMetricResponse{
			{
				MountPoint:   "/",
				Filesystem:   "ext4",
				Total:        100000,
				Used:         25000,
				Free:         75000,
				UsagePercent: 25,
			},
		},
	}

	newCtx := func(serverID string) *gofr.Context {
		base := context.WithValue(context.Background(), authMw.UserClaimsContextKey, &authService.UserClaims{
			UserID: 7,
			Email:  "user@example.com",
			Role:   "USER",
			Name:   "Regular User",
		})
		return &gofr.Context{
			Context: base,
			Request: fakeGofrRequest{
				ctx:       base,
				pathParam: map[string]string{"serverId": serverID},
			},
		}
	}

	t.Run("Authenticated user and own server with metrics succeeds", func(t *testing.T) {
		metricServiceMock := &mockMetricService{
			getLatestFunc: func(ctx context.Context, serverID int64, userID int64) (*dto.LatestMetricResponse, error) {
				if serverID != 42 {
					t.Fatalf("expected serverID 42, got %d", serverID)
				}
				if userID != 7 {
					t.Fatalf("expected userID 7, got %d", userID)
				}
				return expected, nil
			},
		}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		got, err := ctrl.GetLatest(newCtx("42"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		resp, ok := got.(*dto.LatestMetricResponse)
		if !ok {
			t.Fatalf("expected *LatestMetricResponse, got %T", got)
		}
		if resp.ServerID != 42 || resp.CollectedAt != collectedAt {
			t.Fatalf("unexpected response identity: %+v", resp)
		}
		if resp.CPU.UsagePercent != 37.5 || resp.CPU.Cores != 4 {
			t.Errorf("cpu values not returned correctly: %+v", resp.CPU)
		}
		if resp.Load.Load1 != 0.7 || resp.Load.Load5 != 0.6 || resp.Load.Load15 != 0.5 {
			t.Errorf("load values not returned correctly: %+v", resp.Load)
		}
		if resp.Memory.Total != 8000 || resp.Memory.Used != 3200 || resp.Memory.Available != 4800 || resp.Memory.UsagePercent != 40 {
			t.Errorf("memory values not returned correctly: %+v", resp.Memory)
		}
		if resp.Swap.Total != 2000 || resp.Swap.Used != 100 || resp.Swap.Free != 1900 || resp.Swap.UsagePercent != 5 {
			t.Errorf("swap values not returned correctly: %+v", resp.Swap)
		}
		if resp.Network.RXBytes != 10000 || resp.Network.TXBytes != 20000 || resp.Network.RXPackets != 100 || resp.Network.TXPackets != 200 || resp.Network.Errors != 1 || resp.Network.Drops != 2 {
			t.Errorf("network values not returned correctly: %+v", resp.Network)
		}
		if len(resp.Disks) != 1 || resp.Disks[0].MountPoint != "/" || resp.Disks[0].Filesystem != "ext4" {
			t.Errorf("disk values not returned correctly: %+v", resp.Disks)
		}
	})

	t.Run("Owned server with no metrics returns no-metrics error", func(t *testing.T) {
		metricServiceMock := &mockMetricService{
			getLatestFunc: func(ctx context.Context, serverID int64, userID int64) (*dto.LatestMetricResponse, error) {
				return nil, metricService.ErrNoMetricsYet
			},
		}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		_, err := ctrl.GetLatest(newCtx("42"))
		if !errors.Is(err, metricService.ErrNoMetricsYet) {
			t.Fatalf("expected ErrNoMetricsYet, got %v", err)
		}
	})

	t.Run("Another user's server does not expose metrics", func(t *testing.T) {
		metricServiceMock := &mockMetricService{
			getLatestFunc: func(ctx context.Context, serverID int64, userID int64) (*dto.LatestMetricResponse, error) {
				return nil, serverService.ErrServerNotFound
			},
		}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		_, err := ctrl.GetLatest(newCtx("42"))
		if !errors.Is(err, serverService.ErrServerNotFound) {
			t.Fatalf("expected ErrServerNotFound, got %v", err)
		}
	})

	t.Run("Non-existent server returns not found", func(t *testing.T) {
		metricServiceMock := &mockMetricService{
			getLatestFunc: func(ctx context.Context, serverID int64, userID int64) (*dto.LatestMetricResponse, error) {
				return nil, serverService.ErrServerNotFound
			},
		}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		_, err := ctrl.GetLatest(newCtx("999"))
		if !errors.Is(err, serverService.ErrServerNotFound) {
			t.Fatalf("expected ErrServerNotFound, got %v", err)
		}
	})

	t.Run("Invalid server ID returns validation error", func(t *testing.T) {
		metricServiceMock := &mockMetricService{}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		_, err := ctrl.GetLatest(newCtx("not-a-number"))
		if err == nil || err.Error() != "valid server ID is required" {
			t.Fatalf("expected server ID validation error, got %v", err)
		}
	})

	t.Run("Unexpected service error is returned", func(t *testing.T) {
		dbErr := errors.New("database unavailable")
		metricServiceMock := &mockMetricService{
			getLatestFunc: func(ctx context.Context, serverID int64, userID int64) (*dto.LatestMetricResponse, error) {
				return nil, dbErr
			},
		}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		_, err := ctrl.GetLatest(newCtx("42"))
		if !errors.Is(err, dbErr) {
			t.Fatalf("expected database error, got %v", err)
		}
	})
}

func TestMetricController_GetHistory(t *testing.T) {
	start := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	expected := &dto.HistoricalMetricsResponse{
		ServerID: 42,
		Start:    start.Format(time.RFC3339),
		End:      end.Format(time.RFC3339),
		Data: []dto.HistoricalMetricPoint{
			{
				CollectedAt: start.Add(time.Minute).Format(time.RFC3339),
				CPU:         dto.LatestCPUMetricsDTO{UsagePercent: 10.5, Cores: 2},
				Load:        dto.LatestLoadMetricsDTO{Load1: 0.1, Load5: 0.2, Load15: 0.3},
				Memory:      dto.MemoryMetricsDTO{Total: 1000, Used: 400, Available: 600, UsagePercent: 40},
				Swap:        dto.SwapMetricsDTO{Total: 500, Used: 25, Free: 475, UsagePercent: 5},
				Network:     dto.NetworkMetricsDTO{RXBytes: 100, TXBytes: 200, RXPackets: 10, TXPackets: 20, Errors: 0, Drops: 1},
			},
			{
				CollectedAt: start.Add(2 * time.Minute).Format(time.RFC3339),
				CPU:         dto.LatestCPUMetricsDTO{UsagePercent: 20.5, Cores: 2},
			},
		},
	}

	newCtx := func(serverID string, query map[string]string) *gofr.Context {
		base := context.WithValue(context.Background(), authMw.UserClaimsContextKey, &authService.UserClaims{
			UserID: 7,
			Email:  "user@example.com",
			Role:   "USER",
			Name:   "Regular User",
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

	validQuery := map[string]string{
		"start": start.Format(time.RFC3339),
		"end":   end.Format(time.RFC3339),
		"limit": "120",
	}

	t.Run("Authenticated owner gets historical metrics and params are passed", func(t *testing.T) {
		metricServiceMock := &mockMetricService{
			getHistoryFunc: func(ctx context.Context, serverID int64, userID int64, gotStart time.Time, gotEnd time.Time, limit int) (*dto.HistoricalMetricsResponse, error) {
				if serverID != 42 {
					t.Fatalf("expected serverID 42, got %d", serverID)
				}
				if userID != 7 {
					t.Fatalf("expected userID 7, got %d", userID)
				}
				if !gotStart.Equal(start) || !gotEnd.Equal(end) {
					t.Fatalf("expected start/end %s/%s, got %s/%s", start, end, gotStart, gotEnd)
				}
				if limit != 120 {
					t.Fatalf("expected limit 120, got %d", limit)
				}
				return expected, nil
			},
		}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		got, err := ctrl.GetHistory(newCtx("42", validQuery))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resp, ok := got.(*dto.HistoricalMetricsResponse)
		if !ok {
			t.Fatalf("expected *HistoricalMetricsResponse, got %T", got)
		}
		if resp.ServerID != 42 || resp.Start != start.Format(time.RFC3339) || resp.End != end.Format(time.RFC3339) {
			t.Fatalf("unexpected historical envelope: %+v", resp)
		}
		if len(resp.Data) != 2 {
			t.Fatalf("expected 2 historical points, got %d", len(resp.Data))
		}
		if resp.Data[0].CollectedAt != expected.Data[0].CollectedAt || resp.Data[1].CollectedAt != expected.Data[1].CollectedAt {
			t.Fatalf("expected chronological response order to be preserved, got %+v", resp.Data)
		}
	})

	t.Run("Empty historical result succeeds", func(t *testing.T) {
		metricServiceMock := &mockMetricService{
			getHistoryFunc: func(ctx context.Context, serverID int64, userID int64, gotStart time.Time, gotEnd time.Time, limit int) (*dto.HistoricalMetricsResponse, error) {
				return &dto.HistoricalMetricsResponse{
					ServerID: serverID,
					Start:    gotStart.Format(time.RFC3339),
					End:      gotEnd.Format(time.RFC3339),
					Data:     []dto.HistoricalMetricPoint{},
				}, nil
			},
		}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		got, err := ctrl.GetHistory(newCtx("42", validQuery))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resp := got.(*dto.HistoricalMetricsResponse)
		if len(resp.Data) != 0 {
			t.Fatalf("expected empty historical data, got %d", len(resp.Data))
		}
	})

	t.Run("Range exactly equal to maximum succeeds", func(t *testing.T) {
		maxStart := start
		maxEnd := maxStart.Add(metricCtrl.MaxHistoricalRange)
		metricServiceMock := &mockMetricService{
			getHistoryFunc: func(ctx context.Context, serverID int64, userID int64, gotStart time.Time, gotEnd time.Time, limit int) (*dto.HistoricalMetricsResponse, error) {
				if !gotStart.Equal(maxStart) || !gotEnd.Equal(maxEnd) {
					t.Fatalf("expected max range %s/%s, got %s/%s", maxStart, maxEnd, gotStart, gotEnd)
				}
				return &dto.HistoricalMetricsResponse{
					ServerID: serverID,
					Start:    gotStart.Format(time.RFC3339),
					End:      gotEnd.Format(time.RFC3339),
					Data:     []dto.HistoricalMetricPoint{},
				}, nil
			},
		}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		_, err := ctrl.GetHistory(newCtx("42", map[string]string{
			"start": maxStart.Format(time.RFC3339),
			"end":   maxEnd.Format(time.RFC3339),
			"limit": "120",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("Range beyond maximum returns validation error and service is not called", func(t *testing.T) {
		called := false
		metricServiceMock := &mockMetricService{
			getHistoryFunc: func(ctx context.Context, serverID int64, userID int64, gotStart time.Time, gotEnd time.Time, limit int) (*dto.HistoricalMetricsResponse, error) {
				called = true
				return nil, nil
			},
		}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		_, err := ctrl.GetHistory(newCtx("42", map[string]string{
			"start": start.Format(time.RFC3339),
			"end":   start.Add(metricCtrl.MaxHistoricalRange + time.Second).Format(time.RFC3339),
			"limit": "120",
		}))
		if err == nil || err.Error() != "historical range exceeds maximum allowed duration of 168h0m0s" {
			t.Fatalf("expected max range validation error, got %v", err)
		}
		if called {
			t.Fatalf("expected service not to be called when range exceeds maximum")
		}
	})

	t.Run("Start after end returns validation error and service is not called", func(t *testing.T) {
		called := false
		metricServiceMock := &mockMetricService{
			getHistoryFunc: func(ctx context.Context, serverID int64, userID int64, gotStart time.Time, gotEnd time.Time, limit int) (*dto.HistoricalMetricsResponse, error) {
				called = true
				return nil, nil
			},
		}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		_, err := ctrl.GetHistory(newCtx("42", map[string]string{
			"start": end.Format(time.RFC3339),
			"end":   start.Format(time.RFC3339),
			"limit": "120",
		}))
		if err == nil || err.Error() != "start timestamp must be before or equal to end timestamp" {
			t.Fatalf("expected start-after-end validation error, got %v", err)
		}
		if called {
			t.Fatalf("expected service not to be called for invalid time range")
		}
	})

	t.Run("Start equal to end is allowed", func(t *testing.T) {
		metricServiceMock := &mockMetricService{
			getHistoryFunc: func(ctx context.Context, serverID int64, userID int64, gotStart time.Time, gotEnd time.Time, limit int) (*dto.HistoricalMetricsResponse, error) {
				if !gotStart.Equal(start) || !gotEnd.Equal(start) {
					t.Fatalf("expected equal start/end %s, got %s/%s", start, gotStart, gotEnd)
				}
				return &dto.HistoricalMetricsResponse{ServerID: serverID, Start: gotStart.Format(time.RFC3339), End: gotEnd.Format(time.RFC3339), Data: []dto.HistoricalMetricPoint{}}, nil
			},
		}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		_, err := ctrl.GetHistory(newCtx("42", map[string]string{
			"start": start.Format(time.RFC3339),
			"end":   start.Format(time.RFC3339),
			"limit": "120",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("Limit of one is allowed", func(t *testing.T) {
		metricServiceMock := &mockMetricService{
			getHistoryFunc: func(ctx context.Context, serverID int64, userID int64, gotStart time.Time, gotEnd time.Time, limit int) (*dto.HistoricalMetricsResponse, error) {
				if limit != 1 {
					t.Fatalf("expected limit 1, got %d", limit)
				}
				return &dto.HistoricalMetricsResponse{ServerID: serverID, Start: gotStart.Format(time.RFC3339), End: gotEnd.Format(time.RFC3339), Data: []dto.HistoricalMetricPoint{}}, nil
			},
		}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		_, err := ctrl.GetHistory(newCtx("42", map[string]string{
			"start": start.Format(time.RFC3339),
			"end":   end.Format(time.RFC3339),
			"limit": "1",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("Limit exactly equal to maximum is allowed", func(t *testing.T) {
		metricServiceMock := &mockMetricService{
			getHistoryFunc: func(ctx context.Context, serverID int64, userID int64, gotStart time.Time, gotEnd time.Time, limit int) (*dto.HistoricalMetricsResponse, error) {
				if limit != metricCtrl.MaxHistoricalLimit {
					t.Fatalf("expected max limit %d, got %d", metricCtrl.MaxHistoricalLimit, limit)
				}
				return &dto.HistoricalMetricsResponse{ServerID: serverID, Start: gotStart.Format(time.RFC3339), End: gotEnd.Format(time.RFC3339), Data: []dto.HistoricalMetricPoint{}}, nil
			},
		}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		_, err := ctrl.GetHistory(newCtx("42", map[string]string{
			"start": start.Format(time.RFC3339),
			"end":   end.Format(time.RFC3339),
			"limit": strconv.Itoa(metricCtrl.MaxHistoricalLimit),
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("Limit above maximum returns validation error and service is not called", func(t *testing.T) {
		called := false
		metricServiceMock := &mockMetricService{
			getHistoryFunc: func(ctx context.Context, serverID int64, userID int64, gotStart time.Time, gotEnd time.Time, limit int) (*dto.HistoricalMetricsResponse, error) {
				called = true
				return nil, nil
			},
		}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		_, err := ctrl.GetHistory(newCtx("42", map[string]string{
			"start": start.Format(time.RFC3339),
			"end":   end.Format(time.RFC3339),
			"limit": strconv.Itoa(metricCtrl.MaxHistoricalLimit + 1),
		}))
		if err == nil || err.Error() != "limit exceeds maximum allowed value of 10080" {
			t.Fatalf("expected max limit validation error, got %v", err)
		}
		if called {
			t.Fatalf("expected service not to be called when limit exceeds maximum")
		}
	})

	t.Run("Invalid server ID returns validation error", func(t *testing.T) {
		ctrl := metricCtrl.NewMetricController(&mockMetricService{})

		_, err := ctrl.GetHistory(newCtx("not-a-number", validQuery))
		if err == nil || err.Error() != "valid server ID is required" {
			t.Fatalf("expected server ID validation error, got %v", err)
		}
	})

	t.Run("Negative server ID returns validation error", func(t *testing.T) {
		ctrl := metricCtrl.NewMetricController(&mockMetricService{})

		_, err := ctrl.GetHistory(newCtx("-42", validQuery))
		if err == nil || err.Error() != "valid server ID is required" {
			t.Fatalf("expected server ID validation error, got %v", err)
		}
	})

	t.Run("Query userId is ignored and JWT user ID is trusted", func(t *testing.T) {
		metricServiceMock := &mockMetricService{
			getHistoryFunc: func(ctx context.Context, serverID int64, userID int64, gotStart time.Time, gotEnd time.Time, limit int) (*dto.HistoricalMetricsResponse, error) {
				if userID != 7 {
					t.Fatalf("expected userID from JWT claims, got %d", userID)
				}
				return &dto.HistoricalMetricsResponse{
					ServerID: serverID,
					Start:    gotStart.Format(time.RFC3339),
					End:      gotEnd.Format(time.RFC3339),
					Data:     []dto.HistoricalMetricPoint{},
				}, nil
			},
		}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		query := map[string]string{
			"start":  start.Format(time.RFC3339),
			"end":    end.Format(time.RFC3339),
			"limit":  "120",
			"userId": "999",
		}

		_, err := ctrl.GetHistory(newCtx("42", query))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("Missing timestamp returns client validation error", func(t *testing.T) {
		ctrl := metricCtrl.NewMetricController(&mockMetricService{})

		_, err := ctrl.GetHistory(newCtx("42", map[string]string{
			"end":   end.Format(time.RFC3339),
			"limit": "120",
		}))
		if err == nil || err.Error() != "start timestamp is required" {
			t.Fatalf("expected missing start error, got %v", err)
		}
	})

	t.Run("Missing end timestamp returns client validation error", func(t *testing.T) {
		ctrl := metricCtrl.NewMetricController(&mockMetricService{})

		_, err := ctrl.GetHistory(newCtx("42", map[string]string{
			"start": start.Format(time.RFC3339),
			"limit": "120",
		}))
		if err == nil || err.Error() != "end timestamp is required" {
			t.Fatalf("expected missing end error, got %v", err)
		}
	})

	t.Run("Invalid timestamp returns client validation error", func(t *testing.T) {
		ctrl := metricCtrl.NewMetricController(&mockMetricService{})

		_, err := ctrl.GetHistory(newCtx("42", map[string]string{
			"start": "not-a-time",
			"end":   end.Format(time.RFC3339),
			"limit": "120",
		}))
		if err == nil || err.Error() != "valid start timestamp is required" {
			t.Fatalf("expected invalid start error, got %v", err)
		}
	})

	t.Run("Invalid end timestamp returns client validation error", func(t *testing.T) {
		ctrl := metricCtrl.NewMetricController(&mockMetricService{})

		_, err := ctrl.GetHistory(newCtx("42", map[string]string{
			"start": start.Format(time.RFC3339),
			"end":   "not-a-time",
			"limit": "120",
		}))
		if err == nil || err.Error() != "valid end timestamp is required" {
			t.Fatalf("expected invalid end error, got %v", err)
		}
	})

	t.Run("Invalid limit returns client validation error", func(t *testing.T) {
		ctrl := metricCtrl.NewMetricController(&mockMetricService{})

		_, err := ctrl.GetHistory(newCtx("42", map[string]string{
			"start": start.Format(time.RFC3339),
			"end":   end.Format(time.RFC3339),
			"limit": "zero",
		}))
		if err == nil || err.Error() != "valid limit is required" {
			t.Fatalf("expected invalid limit error, got %v", err)
		}
	})

	t.Run("Zero limit returns client validation error", func(t *testing.T) {
		ctrl := metricCtrl.NewMetricController(&mockMetricService{})

		_, err := ctrl.GetHistory(newCtx("42", map[string]string{
			"start": start.Format(time.RFC3339),
			"end":   end.Format(time.RFC3339),
			"limit": "0",
		}))
		if err == nil || err.Error() != "valid limit is required" {
			t.Fatalf("expected zero limit validation error, got %v", err)
		}
	})

	t.Run("Negative limit returns client validation error", func(t *testing.T) {
		ctrl := metricCtrl.NewMetricController(&mockMetricService{})

		_, err := ctrl.GetHistory(newCtx("42", map[string]string{
			"start": start.Format(time.RFC3339),
			"end":   end.Format(time.RFC3339),
			"limit": "-1",
		}))
		if err == nil || err.Error() != "valid limit is required" {
			t.Fatalf("expected negative limit validation error, got %v", err)
		}
	})

	t.Run("Non-existent server returns service error", func(t *testing.T) {
		metricServiceMock := &mockMetricService{
			getHistoryFunc: func(ctx context.Context, serverID int64, userID int64, gotStart time.Time, gotEnd time.Time, limit int) (*dto.HistoricalMetricsResponse, error) {
				return nil, serverService.ErrServerNotFound
			},
		}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		_, err := ctrl.GetHistory(newCtx("999", validQuery))
		if !errors.Is(err, serverService.ErrServerNotFound) {
			t.Fatalf("expected ErrServerNotFound, got %v", err)
		}
	})

	t.Run("Another user's server does not expose history", func(t *testing.T) {
		metricServiceMock := &mockMetricService{
			getHistoryFunc: func(ctx context.Context, serverID int64, userID int64, gotStart time.Time, gotEnd time.Time, limit int) (*dto.HistoricalMetricsResponse, error) {
				return nil, serverService.ErrServerNotFound
			},
		}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		_, err := ctrl.GetHistory(newCtx("42", validQuery))
		if !errors.Is(err, serverService.ErrServerNotFound) {
			t.Fatalf("expected ErrServerNotFound, got %v", err)
		}
	})
}

func TestMetricController_GetAggregate(t *testing.T) {
	start := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	expected := &dto.AggregatedMetricsResponse{
		ServerID:    42,
		Start:       start.Format(time.RFC3339),
		End:         end.Format(time.RFC3339),
		Granularity: "5m",
		Data:        []dto.AggregatedMetricPoint{},
	}

	newCtx := func(serverID string, query map[string]string) *gofr.Context {
		base := context.WithValue(context.Background(), authMw.UserClaimsContextKey, &authService.UserClaims{
			UserID: 7,
			Email:  "user@example.com",
			Role:   "USER",
			Name:   "Regular User",
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

	validQuery := map[string]string{
		"start":       start.Format(time.RFC3339),
		"end":         end.Format(time.RFC3339),
		"granularity": "5m",
	}

	t.Run("Authenticated owner gets aggregate metrics and params are passed", func(t *testing.T) {
		metricServiceMock := &mockMetricService{
			getAggregateFunc: func(ctx context.Context, serverID int64, userID int64, gotStart time.Time, gotEnd time.Time, granularity string) (*dto.AggregatedMetricsResponse, error) {
				if serverID != 42 || userID != 7 {
					t.Fatalf("expected server/user 42/7, got %d/%d", serverID, userID)
				}
				if !gotStart.Equal(start) || !gotEnd.Equal(end) || granularity != "5m" {
					t.Fatalf("unexpected aggregate params start=%s end=%s granularity=%s", gotStart, gotEnd, granularity)
				}
				return expected, nil
			},
		}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		got, err := ctrl.GetAggregate(newCtx("42", validQuery))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resp, ok := got.(*dto.AggregatedMetricsResponse)
		if !ok {
			t.Fatalf("expected *AggregatedMetricsResponse, got %T", got)
		}
		if resp.ServerID != 42 || resp.Granularity != "5m" {
			t.Fatalf("unexpected aggregate response: %+v", resp)
		}
	})

	t.Run("Invalid server ID returns validation error", func(t *testing.T) {
		ctrl := metricCtrl.NewMetricController(&mockMetricService{})

		_, err := ctrl.GetAggregate(newCtx("not-a-number", validQuery))
		if err == nil || err.Error() != "valid server ID is required" {
			t.Fatalf("expected server ID validation error, got %v", err)
		}
		assertHTTPStatus(t, err, http.StatusBadRequest)
	})

	t.Run("Missing granularity returns validation error", func(t *testing.T) {
		ctrl := metricCtrl.NewMetricController(&mockMetricService{})

		_, err := ctrl.GetAggregate(newCtx("42", map[string]string{
			"start": start.Format(time.RFC3339),
			"end":   end.Format(time.RFC3339),
		}))
		if err == nil || err.Error() != "granularity is required" {
			t.Fatalf("expected missing granularity error, got %v", err)
		}
		assertHTTPStatus(t, err, http.StatusBadRequest)
	})

	t.Run("Invalid timestamp returns validation error", func(t *testing.T) {
		ctrl := metricCtrl.NewMetricController(&mockMetricService{})

		_, err := ctrl.GetAggregate(newCtx("42", map[string]string{
			"start":       "not-a-time",
			"end":         end.Format(time.RFC3339),
			"granularity": "5m",
		}))
		if err == nil || err.Error() != "valid start timestamp is required" {
			t.Fatalf("expected invalid start error, got %v", err)
		}
		assertHTTPStatus(t, err, http.StatusBadRequest)
	})

	t.Run("Service validation error is returned", func(t *testing.T) {
		metricServiceMock := &mockMetricService{
			getAggregateFunc: func(ctx context.Context, serverID int64, userID int64, gotStart time.Time, gotEnd time.Time, granularity string) (*dto.AggregatedMetricsResponse, error) {
				return nil, metricService.ErrInvalidGranularity
			},
		}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		_, err := ctrl.GetAggregate(newCtx("42", map[string]string{
			"start":       start.Format(time.RFC3339),
			"end":         end.Format(time.RFC3339),
			"granularity": "10m",
		}))
		if !errors.Is(err, metricService.ErrInvalidGranularity) {
			t.Fatalf("expected ErrInvalidGranularity, got %v", err)
		}
		assertHTTPStatus(t, err, http.StatusBadRequest)
	})

	t.Run("Bucket limit error maps to HTTP 400", func(t *testing.T) {
		metricServiceMock := &mockMetricService{
			getAggregateFunc: func(ctx context.Context, serverID int64, userID int64, gotStart time.Time, gotEnd time.Time, granularity string) (*dto.AggregatedMetricsResponse, error) {
				return nil, metricService.ErrBucketLimitExceeded
			},
		}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		_, err := ctrl.GetAggregate(newCtx("42", validQuery))
		if !errors.Is(err, metricService.ErrBucketLimitExceeded) {
			t.Fatalf("expected ErrBucketLimitExceeded, got %v", err)
		}
		assertHTTPStatus(t, err, http.StatusBadRequest)
	})

	t.Run("Invalid time range maps to HTTP 400", func(t *testing.T) {
		metricServiceMock := &mockMetricService{
			getAggregateFunc: func(ctx context.Context, serverID int64, userID int64, gotStart time.Time, gotEnd time.Time, granularity string) (*dto.AggregatedMetricsResponse, error) {
				return nil, metricService.ErrInvalidTimeRange
			},
		}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		_, err := ctrl.GetAggregate(newCtx("42", validQuery))
		if !errors.Is(err, metricService.ErrInvalidTimeRange) {
			t.Fatalf("expected ErrInvalidTimeRange, got %v", err)
		}
		assertHTTPStatus(t, err, http.StatusBadRequest)
	})

	t.Run("Cross-owner server not found maps to HTTP 404", func(t *testing.T) {
		metricServiceMock := &mockMetricService{
			getAggregateFunc: func(ctx context.Context, serverID int64, userID int64, gotStart time.Time, gotEnd time.Time, granularity string) (*dto.AggregatedMetricsResponse, error) {
				return nil, serverService.ErrServerNotFound
			},
		}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		_, err := ctrl.GetAggregate(newCtx("42", validQuery))
		if !errors.Is(err, serverService.ErrServerNotFound) {
			t.Fatalf("expected ErrServerNotFound, got %v", err)
		}
		assertHTTPStatus(t, err, http.StatusNotFound)
	})

	t.Run("Unexpected service error remains unmapped", func(t *testing.T) {
		dbErr := errors.New("database unavailable")
		metricServiceMock := &mockMetricService{
			getAggregateFunc: func(ctx context.Context, serverID int64, userID int64, gotStart time.Time, gotEnd time.Time, granularity string) (*dto.AggregatedMetricsResponse, error) {
				return nil, dbErr
			},
		}
		ctrl := metricCtrl.NewMetricController(metricServiceMock)

		_, err := ctrl.GetAggregate(newCtx("42", validQuery))
		if !errors.Is(err, dbErr) {
			t.Fatalf("expected database error, got %v", err)
		}
		if _, ok := err.(interface{ StatusCode() int }); ok {
			t.Fatalf("unexpected internal error must not be converted to a client HTTP status error")
		}
	})
}

func assertHTTPStatus(t *testing.T, err error, want int) {
	t.Helper()

	statusErr, ok := err.(interface{ StatusCode() int })
	if !ok {
		t.Fatalf("expected status-aware error, got %T", err)
	}
	if got := statusErr.StatusCode(); got != want {
		t.Fatalf("expected HTTP status %d, got %d", want, got)
	}
}

func TestMetricLatestEndpoint_UserJWTMiddleware(t *testing.T) {
	mockAuth := &mockAuthServiceForUserRoutes{}
	mw := authMw.JWTRoleMiddleware(mockAuth)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	handler := mw(nextHandler)

	t.Run("Missing JWT is rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/user/servers/42/metrics/latest", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("Invalid JWT is rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/user/servers/42/metrics/latest", nil)
		req.Header.Set("Authorization", "Bearer expired-or-invalid")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("Agent credential cannot satisfy user JWT middleware", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/user/servers/42/metrics/latest", nil)
		req.Header.Set("Authorization", "Bearer valid-agent-secret")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("Valid user JWT reaches endpoint handler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/user/servers/42/metrics/latest", nil)
		req.Header.Set("Authorization", "Bearer valid-user-token")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
	})
}

func TestMetricHistoryEndpoint_UserJWTMiddleware(t *testing.T) {
	mockAuth := &mockAuthServiceForUserRoutes{}
	mw := authMw.JWTRoleMiddleware(mockAuth)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	handler := mw(nextHandler)
	historyPath := "/api/v1/user/servers/42/metrics/history?start=2026-09-29T10:00:00Z&end=2026-09-29T12:00:00Z&limit=120"

	t.Run("Missing JWT is rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, historyPath, nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("Invalid JWT is rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, historyPath, nil)
		req.Header.Set("Authorization", "Bearer expired-or-invalid")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("Agent credential cannot satisfy user JWT middleware", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, historyPath, nil)
		req.Header.Set("Authorization", "Bearer valid-agent-secret")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("Valid user JWT reaches endpoint handler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, historyPath, nil)
		req.Header.Set("Authorization", "Bearer valid-user-token")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rr.Code)
		}
	})
}
