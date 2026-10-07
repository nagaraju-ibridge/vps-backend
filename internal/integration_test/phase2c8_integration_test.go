package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	agentDto "vpsmonitoring-backend/internal/agent/dto"
	agentMw "vpsmonitoring-backend/internal/agent/middleware"
	agentModels "vpsmonitoring-backend/internal/agent/models"
	agentRepo "vpsmonitoring-backend/internal/agent/repository"
	agentSvc "vpsmonitoring-backend/internal/agent/service"
	tokenModels "vpsmonitoring-backend/internal/installation_token/models"
	tokenSvc "vpsmonitoring-backend/internal/installation_token/service"
	metricDto "vpsmonitoring-backend/internal/metric/dto"
	metricModels "vpsmonitoring-backend/internal/metric/models"
	metricRepo "vpsmonitoring-backend/internal/metric/repository"
	metricSvc "vpsmonitoring-backend/internal/metric/service"
	serverModels "vpsmonitoring-backend/internal/server/models"
)

// inMemoryIntegrationDB simulates backend database state for full integration pipeline testing
type inMemoryIntegrationDB struct {
	mu           sync.Mutex
	servers      map[int64]*serverModels.Server
	tokens       map[string]*tokenModels.InstallationToken
	agents       map[string]*agentModels.Agent // keyed by token_hash
	metrics      []*metricModels.Metric
	metricDisks  map[uuid.UUID][]metricModels.MetricDisk
	nextServerID int64
	nextAgentID  int64
	nextDiskID   int64
}

func newInMemoryIntegrationDB() *inMemoryIntegrationDB {
	return &inMemoryIntegrationDB{
		servers:      make(map[int64]*serverModels.Server),
		tokens:       make(map[string]*tokenModels.InstallationToken),
		agents:       make(map[string]*agentModels.Agent),
		metricDisks:  make(map[uuid.UUID][]metricModels.MetricDisk),
		nextServerID: 1,
		nextAgentID:  1,
		nextDiskID:   1,
	}
}

// mockTokenHasher implements token hashing
type mockTokenHasher struct {
	tokenSvc.InstallationTokenService
}

func (m *mockTokenHasher) HashToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

// mockAgentRepo implements agentRepo.AgentRepository on inMemoryIntegrationDB
type mockAgentRepo struct {
	db *inMemoryIntegrationDB
}

func (r *mockAgentRepo) RegisterAgentTx(ctx context.Context, params agentRepo.RegistrationParams) (*agentModels.Agent, int64, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()

	tok, exists := r.db.tokens[params.TokenHash]
	if !exists {
		return nil, 0, agentRepo.ErrTokenNotFound
	}
	if tok.IsUsed {
		return nil, 0, agentRepo.ErrTokenAlreadyUsed
	}
	if tok.IsExpired() {
		return nil, 0, agentRepo.ErrTokenExpired
	}

	srv, exists := r.db.servers[tok.ServerID]
	if !exists || srv.DeletedAt.Valid {
		return nil, 0, agentRepo.ErrServerNotFound
	}

	now := time.Now().UTC()
	tok.IsUsed = true
	tok.UsedAt = &now

	srv.AgentID = &params.AgentID
	srv.AgentStatus = serverModels.StatusOnline
	srv.Hostname = &params.Hostname
	srv.IPAddress = &params.IPAddress
	srv.OS = &params.OS
	srv.Architecture = &params.Architecture
	srv.UpdatedAt = now

	agent := &agentModels.Agent{
		ID:        r.db.nextAgentID,
		ServerID:  tok.ServerID,
		AgentID:   params.AgentID,
		TokenHash: params.CredentialHash,
		Version:   params.AgentVersion,
		Status:    agentModels.AgentStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	r.db.nextAgentID++
	r.db.agents[params.CredentialHash] = agent

	return agent, tok.ServerID, nil
}

func (r *mockAgentRepo) GetByAgentID(ctx context.Context, agentID string) (*agentModels.Agent, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	for _, a := range r.db.agents {
		if a.AgentID == agentID {
			return a, nil
		}
	}
	return nil, nil
}

func (r *mockAgentRepo) GetByServerID(ctx context.Context, serverID int64) (*agentModels.Agent, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	for _, a := range r.db.agents {
		if a.ServerID == serverID {
			return a, nil
		}
	}
	return nil, nil
}

func (r *mockAgentRepo) GetByCredentialHash(ctx context.Context, credentialHash string) (*agentModels.Agent, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	agent, exists := r.db.agents[credentialHash]
	if !exists {
		return nil, nil
	}
	return agent, nil
}

func (r *mockAgentRepo) UpdateHeartbeat(ctx context.Context, agentID string, serverID int64, timestamp time.Time, agentVersion string) error {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()

	var targetAgent *agentModels.Agent
	for _, a := range r.db.agents {
		if a.AgentID == agentID && a.ServerID == serverID {
			targetAgent = a
			break
		}
	}
	if targetAgent == nil {
		return agentRepo.ErrAgentNotFound
	}
	if targetAgent.Status != agentModels.AgentStatusActive {
		return agentRepo.ErrAgentInactive
	}

	srv, exists := r.db.servers[serverID]
	if !exists || srv.DeletedAt.Valid {
		return agentRepo.ErrServerNotFound
	}

	targetAgent.LastSeen = &timestamp
	targetAgent.UpdatedAt = timestamp
	if agentVersion != "" {
		targetAgent.Version = agentVersion
	}

	srv.LastSeen = &timestamp
	srv.AgentStatus = serverModels.StatusOnline
	srv.UpdatedAt = timestamp

	return nil
}

func (r *mockAgentRepo) MarkStaleServersOffline(ctx context.Context, threshold time.Time) (int64, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()

	var count int64
	for _, srv := range r.db.servers {
		if srv.DeletedAt.Valid {
			continue
		}
		if srv.AgentStatus == serverModels.StatusOnline && srv.LastSeen != nil && srv.LastSeen.Before(threshold) {
			srv.AgentStatus = serverModels.StatusOffline
			count++
		}
	}
	return count, nil
}

func (r *mockAgentRepo) AutoMigrate() error {
	return nil
}

// mockMetricRepo implements metricRepo.MetricRepository on inMemoryIntegrationDB
type mockMetricRepo struct {
	db *inMemoryIntegrationDB
}

func (r *mockMetricRepo) AutoMigrate() error {
	return nil
}

func (r *mockMetricRepo) CreateMetric(ctx context.Context, metric *metricModels.Metric, disks []metricModels.MetricDisk) error {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()

	if metric == nil {
		return errors.New("metric cannot be nil")
	}

	if metric.ID == uuid.Nil {
		metric.ID = uuid.New()
	}

	r.db.metrics = append(r.db.metrics, metric)
	if len(disks) > 0 {
		for i := range disks {
			if disks[i].ID == uuid.Nil {
				disks[i].ID = uuid.New()
			}
			disks[i].MetricID = metric.ID
		}
		r.db.metricDisks[metric.ID] = disks
	}

	return nil
}

func (r *mockMetricRepo) GetLatestByServerID(ctx context.Context, serverID int64) (*metricModels.Metric, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()

	var latest *metricModels.Metric
	for _, m := range r.db.metrics {
		if m.ServerID == serverID {
			if latest == nil || m.CollectedAt.After(latest.CollectedAt) {
				latest = m
			}
		}
	}
	if latest != nil {
		latest.Disks = r.db.metricDisks[latest.ID]
	}
	return latest, nil
}

func (r *mockMetricRepo) GetHistoryByServerID(ctx context.Context, serverID int64, start time.Time, end time.Time, limit int) ([]metricModels.Metric, error) {
	return nil, nil
}

func (r *mockMetricRepo) GetAggregatedMetrics(ctx context.Context, serverID int64, start time.Time, end time.Time, granularity string) ([]metricRepo.AggregatedMetricRow, []metricRepo.AggregatedDiskRow, error) {
	return nil, nil, nil
}

// Setup full backend HTTP handler simulating real router and middlewares
func setupIntegrationBackend(db *inMemoryIntegrationDB) http.Handler {
	hasher := &mockTokenHasher{}
	agentRepository := &mockAgentRepo{db: db}
	agentService := agentSvc.NewAgentService(agentRepository, hasher)

	metricRepository := &mockMetricRepo{db: db}
	metricService := metricSvc.NewMetricService(metricRepository, nil)

	mux := http.NewServeMux()

	// Public registration
	mux.HandleFunc("POST /api/v1/agent/register", func(w http.ResponseWriter, r *http.Request) {
		var req agentDto.RegisterAgentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":{"message":"invalid request body"}}`, http.StatusBadRequest)
			return
		}
		resp, err := agentService.Register(r.Context(), req)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":{"message":"%s"}}`, err.Error()), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": resp})
	})

	// Heartbeat protected by AgentAuthMiddleware
	mux.HandleFunc("POST /api/v1/agent/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		val := r.Context().Value(agentMw.AgentIdentityContextKey)
		if val == nil {
			http.Error(w, `{"error":{"message":"unauthenticated"}}`, http.StatusUnauthorized)
			return
		}
		identity := val.(*agentMw.AgentIdentity)

		var req agentDto.HeartbeatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":{"message":"invalid request body"}}`, http.StatusBadRequest)
			return
		}

		resp, err := agentService.Heartbeat(r.Context(), identity.AgentID, identity.ServerID, req)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":{"message":"%s"}}`, err.Error()), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": resp})
	})

	// Metrics protected by AgentAuthMiddleware
	mux.HandleFunc("POST /api/v1/agent/metrics", func(w http.ResponseWriter, r *http.Request) {
		val := r.Context().Value(agentMw.AgentIdentityContextKey)
		if val == nil {
			http.Error(w, `{"error":{"message":"unauthenticated"}}`, http.StatusUnauthorized)
			return
		}
		identity := val.(*agentMw.AgentIdentity)

		var req metricDto.IngestMetricRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":{"message":"invalid request body"}}`, http.StatusBadRequest)
			return
		}

		resp, err := metricService.IngestMetric(r.Context(), identity.AgentID, identity.ServerID, req)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":{"message":"%s"}}`, err.Error()), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": resp})
	})

	// Wrap mux in AgentAuthMiddleware
	return agentMw.AgentAuthMiddleware(agentService)(mux)
}

func TestPhase2C8_AgentBackendIntegration(t *testing.T) {
	db := newInMemoryIntegrationDB()
	hasher := &mockTokenHasher{}

	// Setup initial test Server in PENDING state
	serverID := int64(101)
	db.servers[serverID] = &serverModels.Server{
		ID:          serverID,
		UserID:      1,
		Name:        "production-web-01",
		AgentStatus: serverModels.StatusPending,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	// Setup initial active Installation Token
	rawToken := "vpspulse_install_token_valid_12345"
	tokenHash := hasher.HashToken(rawToken)
	db.tokens[tokenHash] = &tokenModels.InstallationToken{
		ID:        1,
		ServerID:  serverID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().UTC().Add(24 * time.Hour),
		IsUsed:    false,
		CreatedAt: time.Now().UTC(),
	}

	backendHandler := setupIntegrationBackend(db)
	backendServer := httptest.NewServer(backendHandler)
	defer backendServer.Close()

	var agentCredential string
	var registeredAgentID string

	// ----------------------------------------------------
	// 1. Real Go Agent Registration (Item 1 & 4: Server ONLINE)
	// ----------------------------------------------------
	t.Run("1. Real Agent Registration & Server becomes ONLINE", func(t *testing.T) {
		regPayload := agentDto.RegisterAgentRequest{
			InstallationToken: rawToken,
			Hostname:          "web01.internal",
			IPAddress:         "192.168.1.50",
			OS:                "linux",
			Architecture:      "amd64",
			AgentVersion:      "0.1.0",
		}
		body, _ := json.Marshal(regPayload)

		resp, err := http.Post(backendServer.URL+"/api/v1/agent/register", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("registration HTTP request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}

		var regResp struct {
			Data agentDto.RegisterAgentResponse `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&regResp); err != nil {
			t.Fatalf("failed to decode registration response: %v", err)
		}

		if regResp.Data.Credential == "" {
			t.Fatalf("expected non-empty agent credential")
		}
		if regResp.Data.ServerID != serverID {
			t.Fatalf("expected ServerID %d, got %d", serverID, regResp.Data.ServerID)
		}

		agentCredential = regResp.Data.Credential
		registeredAgentID = regResp.Data.AgentID

		// Verify Token is consumed
		if !db.tokens[tokenHash].IsUsed {
			t.Errorf("expected installation token to be marked as used")
		}
		if db.tokens[tokenHash].UsedAt == nil {
			t.Errorf("expected token used_at to be populated")
		}

		// Verify Server is ONLINE
		if db.servers[serverID].AgentStatus != serverModels.StatusOnline {
			t.Errorf("expected server to transition to ONLINE, got %s", db.servers[serverID].AgentStatus)
		}
		if *db.servers[serverID].AgentID != registeredAgentID {
			t.Errorf("expected server agent_id %s, got %s", registeredAgentID, *db.servers[serverID].AgentID)
		}
	})

	// ----------------------------------------------------
	// 2. Real Heartbeat (Item 3 & 4: Continuous Heartbeat & Status)
	// ----------------------------------------------------
	t.Run("2. Real Heartbeat keeps Server ONLINE and updates last_seen", func(t *testing.T) {
		hbPayload := agentDto.HeartbeatRequest{
			Timestamp:    time.Now().UTC().Format(time.RFC3339),
			AgentVersion: "0.1.0",
		}
		body, _ := json.Marshal(hbPayload)

		req, _ := http.NewRequest(http.MethodPost, backendServer.URL+"/api/v1/agent/heartbeat", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+agentCredential)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("heartbeat HTTP request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}

		// Verify Server last_seen updated
		if db.servers[serverID].LastSeen == nil {
			t.Fatalf("expected server last_seen to be set")
		}
		if db.servers[serverID].AgentStatus != serverModels.StatusOnline {
			t.Errorf("expected server to remain ONLINE")
		}
	})

	// ----------------------------------------------------
	// 3. Real Metrics Reach Backend & Stored (Items 5 & 6)
	// ----------------------------------------------------
	t.Run("3. Metric Ingestion stores CPU, Memory, Swap, Network, and MountPoints", func(t *testing.T) {
		now := time.Now().UTC()
		metricPayload := metricDto.IngestMetricRequest{
			Timestamp: now,
			CPU: metricDto.CPUMetricsDTO{
				UsagePercent: 28.5,
				Cores:        4,
				Load1:        0.75,
				Load5:        0.55,
				Load15:       0.40,
			},
			Memory: metricDto.MemoryMetricsDTO{
				Total:        16000000000,
				Used:         8000000000,
				Available:    8000000000,
				UsagePercent: 50.0,
			},
			Swap: metricDto.SwapMetricsDTO{
				Total:        2000000000,
				Used:         200000000,
				Free:         1800000000,
				UsagePercent: 10.0,
			},
			Disk: metricDto.DiskMetricsDTO{
				MountPoints: []metricDto.MountPointDTO{
					{Path: "/", FSType: "ext4", Total: 100000000000, Used: 40000000000, Free: 60000000000, UsagePercent: 40.0},
					{Path: "/data", FSType: "ext4", Total: 500000000000, Used: 100000000000, Free: 400000000000, UsagePercent: 20.0},
				},
			},
			Network: metricDto.NetworkMetricsDTO{
				RXBytes:   100000,
				TXBytes:   200000,
				RXPackets: 1000,
				TXPackets: 2000,
				Errors:    0,
				Drops:     0,
			},
		}
		body, _ := json.Marshal(metricPayload)

		req, _ := http.NewRequest(http.MethodPost, backendServer.URL+"/api/v1/agent/metrics", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+agentCredential)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("metrics HTTP request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}

		if len(db.metrics) != 1 {
			t.Fatalf("expected 1 metric snapshot in DB, found %d", len(db.metrics))
		}

		stored := db.metrics[0]
		if stored.ServerID != serverID {
			t.Errorf("expected ServerID %d, got %d", serverID, stored.ServerID)
		}
		if stored.AgentID != registeredAgentID {
			t.Errorf("expected AgentID %s, got %s", registeredAgentID, stored.AgentID)
		}
		if stored.CPUUsagePercent != 28.5 {
			t.Errorf("expected CPU 28.5, got %f", stored.CPUUsagePercent)
		}
		if stored.MemoryTotal != 16000000000 {
			t.Errorf("expected MemoryTotal 16000000000, got %d", stored.MemoryTotal)
		}

		disks := db.metricDisks[stored.ID]
		if len(disks) != 2 {
			t.Fatalf("expected 2 disk mount points stored, got %d", len(disks))
		}
		if disks[0].MountPoint != "/" || disks[1].MountPoint != "/data" {
			t.Errorf("unexpected mount points: %+v", disks)
		}
	})

	// ----------------------------------------------------
	// 4. Single-Use Token Enforcement (Item 14)
	// ----------------------------------------------------
	t.Run("4. Second registration using same token is rejected", func(t *testing.T) {
		regPayload := agentDto.RegisterAgentRequest{
			InstallationToken: rawToken, // Already used!
			Hostname:          "web01-clone",
		}
		body, _ := json.Marshal(regPayload)

		resp, err := http.Post(backendServer.URL+"/api/v1/agent/register", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			t.Fatalf("expected used token to be rejected, got 200 OK")
		}
	})

	// ----------------------------------------------------
	// 5. Expired Token Enforcement (Item 10)
	// ----------------------------------------------------
	t.Run("5. Expired installation token is rejected", func(t *testing.T) {
		expiredRawToken := "vpspulse_install_token_expired_999"
		expiredHash := hasher.HashToken(expiredRawToken)
		db.tokens[expiredHash] = &tokenModels.InstallationToken{
			ID:        999,
			ServerID:  serverID,
			TokenHash: expiredHash,
			ExpiresAt: time.Now().UTC().Add(-2 * time.Hour), // Expired!
			IsUsed:    false,
		}

		regPayload := agentDto.RegisterAgentRequest{
			InstallationToken: expiredRawToken,
			Hostname:          "web01-expired",
		}
		body, _ := json.Marshal(regPayload)

		resp, err := http.Post(backendServer.URL+"/api/v1/agent/register", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			t.Fatalf("expected expired token to be rejected, got 200 OK")
		}
	})

	// ----------------------------------------------------
	// 6. Invalid Credential Handling (Item 9)
	// ----------------------------------------------------
	t.Run("6. Invalid credentials return 401 Unauthorized for heartbeat and metrics", func(t *testing.T) {
		// Heartbeat with fake cred
		reqHB, _ := http.NewRequest(http.MethodPost, backendServer.URL+"/api/v1/agent/heartbeat", bytes.NewReader([]byte(`{}`)))
		reqHB.Header.Set("Authorization", "Bearer invalid-cred-12345")
		reqHB.Header.Set("Content-Type", "application/json")

		respHB, _ := http.DefaultClient.Do(reqHB)
		if respHB.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 for invalid heartbeat, got %d", respHB.StatusCode)
		}

		// Metrics with fake cred
		reqM, _ := http.NewRequest(http.MethodPost, backendServer.URL+"/api/v1/agent/metrics", bytes.NewReader([]byte(`{"timestamp":"`+time.Now().UTC().Format(time.RFC3339)+`"}`)))
		reqM.Header.Set("Authorization", "Bearer invalid-cred-12345")
		reqM.Header.Set("Content-Type", "application/json")

		respM, _ := http.DefaultClient.Do(reqM)
		if respM.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 for invalid metrics, got %d", respM.StatusCode)
		}
	})

	// ----------------------------------------------------
	// 7. Offline Detection Transition (Item 4 & 5: Online -> Offline -> Online)
	// ----------------------------------------------------
	t.Run("7. Offline detection transition Online -> Offline -> Online", func(t *testing.T) {
		repo := &mockAgentRepo{db: db}

		// Simulate passage of time beyond threshold
		threshold := time.Now().UTC().Add(10 * time.Minute)
		count, err := repo.MarkStaleServersOffline(context.Background(), threshold)
		if err != nil {
			t.Fatalf("failed to run offline detection: %v", err)
		}
		if count != 1 {
			t.Errorf("expected 1 server marked offline, got %d", count)
		}
		if db.servers[serverID].AgentStatus != serverModels.StatusOffline {
			t.Errorf("expected server status OFFLINE, got %s", db.servers[serverID].AgentStatus)
		}

		// Agent sends heartbeat after restarting/reconnecting -> transitions back to ONLINE
		hbPayload := agentDto.HeartbeatRequest{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}
		body, _ := json.Marshal(hbPayload)
		req, _ := http.NewRequest(http.MethodPost, backendServer.URL+"/api/v1/agent/heartbeat", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+agentCredential)
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("heartbeat recovery failed: %v, status: %d", err, resp.StatusCode)
		}

		if db.servers[serverID].AgentStatus != serverModels.StatusOnline {
			t.Errorf("expected server to recover to ONLINE, got %s", db.servers[serverID].AgentStatus)
		}
	})
}
