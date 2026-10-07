package service_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	agentDto "vpsmonitoring-backend/internal/agent/dto"
	agentModels "vpsmonitoring-backend/internal/agent/models"
	agentRepo "vpsmonitoring-backend/internal/agent/repository"
	agentService "vpsmonitoring-backend/internal/agent/service"
	tokenModels "vpsmonitoring-backend/internal/installation_token/models"
	tokenService "vpsmonitoring-backend/internal/installation_token/service"
	serverModels "vpsmonitoring-backend/internal/server/models"
)

// mockTokenHasher implements tokenService.InstallationTokenService for testing
type mockTokenHasher struct {
	tokenService.InstallationTokenService
}

func (m *mockTokenHasher) HashToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

// mockAgentRepository implements agentRepo.AgentRepository with transaction simulation
type mockAgentRepository struct {
	agents       map[int64]*agentModels.Agent
	servers      map[int64]*serverModels.Server
	tokens       map[string]*tokenModels.InstallationToken
	failTxOnStep string // simulate failure at specific step
	nextAgentID  int64
}

func newMockAgentRepository() *mockAgentRepository {
	return &mockAgentRepository{
		agents:      make(map[int64]*agentModels.Agent),
		servers:     make(map[int64]*serverModels.Server),
		tokens:      make(map[string]*tokenModels.InstallationToken),
		nextAgentID: 1,
	}
}

func (m *mockAgentRepository) RegisterAgentTx(ctx context.Context, params agentRepo.RegistrationParams) (*agentModels.Agent, int64, error) {
	// 1. Token validation
	tok, exists := m.tokens[params.TokenHash]
	if !exists {
		return nil, 0, agentRepo.ErrTokenNotFound
	}
	if tok.IsUsed {
		return nil, 0, agentRepo.ErrTokenAlreadyUsed
	}
	if tok.ExpiresAt.Before(time.Now()) {
		return nil, 0, agentRepo.ErrTokenExpired
	}

	serverID := tok.ServerID

	// 2. Server validation
	srv, exists := m.servers[serverID]
	if !exists || srv.DeletedAt.Valid {
		return nil, 0, agentRepo.ErrServerNotFound
	}

	// Simulated failure before commit for transaction rollback testing
	if m.failTxOnStep == "simulate_rollback" {
		return nil, 0, errors.New("simulated database transaction error")
	}

	// 3. Create or update Agent
	var agent *agentModels.Agent
	for _, a := range m.agents {
		if a.ServerID == serverID {
			agent = a
			break
		}
	}

	now := time.Now()
	if agent == nil {
		agent = &agentModels.Agent{
			ID:        m.nextAgentID,
			ServerID:  serverID,
			AgentID:   params.AgentID,
			TokenHash: params.CredentialHash,
			Version:   params.AgentVersion,
			Status:    agentModels.AgentStatusActive,
			CreatedAt: now,
			UpdatedAt: now,
		}
		m.nextAgentID++
		m.agents[agent.ID] = agent
	} else {
		// Update existing
		agent.AgentID = params.AgentID
		agent.TokenHash = params.CredentialHash
		agent.Version = params.AgentVersion
		agent.Status = agentModels.AgentStatusActive
		agent.UpdatedAt = now
	}

	// 4. Update Server
	srv.Hostname = &params.Hostname
	srv.IPAddress = &params.IPAddress
	srv.OS = &params.OS
	srv.Architecture = &params.Architecture
	srv.AgentID = &params.AgentID
	srv.AgentStatus = serverModels.StatusOnline

	// 5. Consume token
	tok.IsUsed = true
	tok.UsedAt = &now

	savedAgent := *agent
	return &savedAgent, serverID, nil
}

func (m *mockAgentRepository) GetByAgentID(ctx context.Context, agentID string) (*agentModels.Agent, error) {
	for _, a := range m.agents {
		if a.AgentID == agentID {
			cp := *a
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockAgentRepository) GetByServerID(ctx context.Context, serverID int64) (*agentModels.Agent, error) {
	for _, a := range m.agents {
		if a.ServerID == serverID {
			cp := *a
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockAgentRepository) GetByCredentialHash(ctx context.Context, credentialHash string) (*agentModels.Agent, error) {
	for _, a := range m.agents {
		if a.TokenHash == credentialHash {
			cp := *a
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockAgentRepository) UpdateHeartbeat(ctx context.Context, agentID string, serverID int64, timestamp time.Time, agentVersion string) error {
	var targetAgent *agentModels.Agent
	for _, a := range m.agents {
		if a.AgentID == agentID {
			targetAgent = a
			break
		}
	}
	if targetAgent == nil || targetAgent.ServerID != serverID {
		return agentRepo.ErrAgentNotFound
	}
	if targetAgent.Status != agentModels.AgentStatusActive {
		return agentRepo.ErrAgentInactive
	}

	srv, exists := m.servers[serverID]
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

func (m *mockAgentRepository) MarkStaleServersOffline(ctx context.Context, threshold time.Time) (int64, error) {
	var count int64
	for _, srv := range m.servers {
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

func (m *mockAgentRepository) AutoMigrate() error {
	return nil
}

func TestAgentService_SuccessfulRegistration(t *testing.T) {
	repo := newMockAgentRepository()
	hasher := &mockTokenHasher{}
	svc := agentService.NewAgentService(repo, hasher)
	ctx := context.Background()

	// Seed server 1
	repo.servers[1] = &serverModels.Server{
		ID:          1,
		UserID:      10,
		Name:        "Production Ubuntu VPS",
		AgentStatus: serverModels.StatusPending,
	}

	// Seed installation token for server 1
	rawToken := "raw-install-token-12345"
	tokenHash := hasher.HashToken(rawToken)
	repo.tokens[tokenHash] = &tokenModels.InstallationToken{
		ID:        1,
		ServerID:  1,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		IsUsed:    false,
	}

	req := agentDto.RegisterAgentRequest{
		InstallationToken: rawToken,
		Hostname:          "prod-vps-01",
		IPAddress:         "192.168.1.100",
		OS:                "Ubuntu 24.04 LTS",
		Architecture:      "x86_64",
		AgentVersion:      "0.1.0",
	}

	res, err := svc.Register(ctx, req)
	if err != nil {
		t.Fatalf("expected registration to succeed, got error: %v", err)
	}

	// 1. Response validations
	if res.ServerID != 1 {
		t.Errorf("expected server_id 1, got %d", res.ServerID)
	}
	if res.Status != agentModels.AgentStatusActive {
		t.Errorf("expected agent status ACTIVE, got %s", res.Status)
	}
	if len(res.AgentID) == 0 {
		t.Errorf("expected non-empty UUID agent_id")
	}
	if len(res.Credential) != 64 {
		t.Errorf("expected 64-char raw credential, got length %d", len(res.Credential))
	}

	// 2. Server state verification
	srv := repo.servers[1]
	if srv.AgentStatus != serverModels.StatusOnline {
		t.Errorf("expected server status transitioned to ONLINE, got %s", srv.AgentStatus)
	}
	if *srv.Hostname != "prod-vps-01" {
		t.Errorf("expected hostname prod-vps-01, got %s", *srv.Hostname)
	}
	if *srv.IPAddress != "192.168.1.100" {
		t.Errorf("expected ip_address 192.168.1.100, got %s", *srv.IPAddress)
	}
	if *srv.OS != "Ubuntu 24.04 LTS" {
		t.Errorf("expected OS Ubuntu 24.04 LTS, got %s", *srv.OS)
	}
	if *srv.Architecture != "x86_64" {
		t.Errorf("expected architecture x86_64, got %s", *srv.Architecture)
	}
	if *srv.AgentID != res.AgentID {
		t.Errorf("expected server.agent_id to match returned agent_id")
	}

	// 3. Token consumption verification
	tok := repo.tokens[tokenHash]
	if !tok.IsUsed {
		t.Errorf("expected installation token to be marked is_used=true")
	}
	if tok.UsedAt == nil {
		t.Errorf("expected used_at timestamp to be recorded")
	}

	// 4. Credential security verification
	agent := repo.agents[1]
	if agent.TokenHash == res.Credential {
		t.Errorf("SECURITY FLAW: raw credential matches stored hash in database")
	}
	expectedCredHash := hasher.HashToken(res.Credential)
	if agent.TokenHash != expectedCredHash {
		t.Errorf("expected stored hash %s, got %s", expectedCredHash, agent.TokenHash)
	}
	if agent.LastSeen != nil {
		t.Errorf("last_seen must remain NULL because heartbeat is not implemented yet")
	}
}

func TestAgentService_TokenRejections(t *testing.T) {
	repo := newMockAgentRepository()
	hasher := &mockTokenHasher{}
	svc := agentService.NewAgentService(repo, hasher)
	ctx := context.Background()

	repo.servers[1] = &serverModels.Server{ID: 1, UserID: 10, Name: "VPS 1"}

	// 1. Missing Token Rejection
	_, err := svc.Register(ctx, agentDto.RegisterAgentRequest{InstallationToken: "   "})
	if !errors.Is(err, agentService.ErrMissingInstallationToken) {
		t.Errorf("expected ErrMissingInstallationToken, got %v", err)
	}

	// 2. Invalid/Nonexistent Token Rejection
	_, err = svc.Register(ctx, agentDto.RegisterAgentRequest{InstallationToken: "bogus-token"})
	if !errors.Is(err, agentService.ErrInvalidInstallationToken) {
		t.Errorf("expected ErrInvalidInstallationToken, got %v", err)
	}

	// 3. Expired Token Rejection
	expiredRaw := "expired-token-val"
	expiredHash := hasher.HashToken(expiredRaw)
	repo.tokens[expiredHash] = &tokenModels.InstallationToken{
		ID:        2,
		ServerID:  1,
		TokenHash: expiredHash,
		ExpiresAt: time.Now().Add(-10 * time.Minute),
		IsUsed:    false,
	}

	_, err = svc.Register(ctx, agentDto.RegisterAgentRequest{InstallationToken: expiredRaw})
	if !errors.Is(err, agentService.ErrTokenExpired) {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}

	// 4. Already Used Token Rejection
	usedRaw := "already-used-token-val"
	usedHash := hasher.HashToken(usedRaw)
	repo.tokens[usedHash] = &tokenModels.InstallationToken{
		ID:        3,
		ServerID:  1,
		TokenHash: usedHash,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		IsUsed:    true,
	}

	_, err = svc.Register(ctx, agentDto.RegisterAgentRequest{InstallationToken: usedRaw})
	if !errors.Is(err, agentService.ErrTokenAlreadyUsed) {
		t.Errorf("expected ErrTokenAlreadyUsed, got %v", err)
	}
}

func TestAgentService_DeletedServerRejection(t *testing.T) {
	repo := newMockAgentRepository()
	hasher := &mockTokenHasher{}
	svc := agentService.NewAgentService(repo, hasher)
	ctx := context.Background()

	// Soft-deleted server
	repo.servers[2] = &serverModels.Server{
		ID:        2,
		UserID:    10,
		Name:      "Deleted VPS",
		DeletedAt: gorm.DeletedAt{Time: time.Now(), Valid: true},
	}

	validRaw := "token-for-deleted-server"
	tokenHash := hasher.HashToken(validRaw)
	repo.tokens[tokenHash] = &tokenModels.InstallationToken{
		ID:        4,
		ServerID:  2,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		IsUsed:    false,
	}

	_, err := svc.Register(ctx, agentDto.RegisterAgentRequest{InstallationToken: validRaw})
	if !errors.Is(err, agentService.ErrServerNotFound) {
		t.Errorf("expected ErrServerNotFound for deleted server, got %v", err)
	}
}

func TestAgentService_DuplicateRegistration_UpdatesAgent(t *testing.T) {
	repo := newMockAgentRepository()
	hasher := &mockTokenHasher{}
	svc := agentService.NewAgentService(repo, hasher)
	ctx := context.Background()

	repo.servers[1] = &serverModels.Server{ID: 1, UserID: 10, Name: "VPS 1"}

	// First registration
	tok1 := "token-one-1111"
	hash1 := hasher.HashToken(tok1)
	repo.tokens[hash1] = &tokenModels.InstallationToken{
		ID:        1,
		ServerID:  1,
		TokenHash: hash1,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		IsUsed:    false,
	}

	res1, err := svc.Register(ctx, agentDto.RegisterAgentRequest{
		InstallationToken: tok1,
		Hostname:          "host-first",
		IPAddress:         "10.0.0.1",
	})
	if err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	// Second registration with a fresh new token (re-installation)
	tok2 := "token-two-2222"
	hash2 := hasher.HashToken(tok2)
	repo.tokens[hash2] = &tokenModels.InstallationToken{
		ID:        2,
		ServerID:  1,
		TokenHash: hash2,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		IsUsed:    false,
	}

	res2, err := svc.Register(ctx, agentDto.RegisterAgentRequest{
		InstallationToken: tok2,
		Hostname:          "host-second",
		IPAddress:         "10.0.0.2",
	})
	if err != nil {
		t.Fatalf("second registration failed: %v", err)
	}

	// Verify agent was updated with new agent_id and new credential
	if res1.AgentID == res2.AgentID {
		t.Errorf("expected distinct agent_id upon re-registration")
	}
	if res1.Credential == res2.Credential {
		t.Errorf("expected distinct credential upon re-registration")
	}
	if *repo.servers[1].Hostname != "host-second" {
		t.Errorf("expected updated hostname host-second")
	}
}

func TestAgentService_TransactionRollback(t *testing.T) {
	repo := newMockAgentRepository()
	hasher := &mockTokenHasher{}
	svc := agentService.NewAgentService(repo, hasher)
	ctx := context.Background()

	repo.servers[1] = &serverModels.Server{ID: 1, UserID: 10, Name: "VPS 1"}

	rawToken := "rollback-test-token"
	tokenHash := hasher.HashToken(rawToken)
	repo.tokens[tokenHash] = &tokenModels.InstallationToken{
		ID:        5,
		ServerID:  1,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		IsUsed:    false,
	}

	// Instruct mock to fail inside transaction
	repo.failTxOnStep = "simulate_rollback"

	_, err := svc.Register(ctx, agentDto.RegisterAgentRequest{InstallationToken: rawToken})
	if err == nil || !strings.Contains(err.Error(), "simulated database transaction error") {
		t.Fatalf("expected simulated transaction failure, got %v", err)
	}

	// Verify token was NOT consumed
	if repo.tokens[tokenHash].IsUsed {
		t.Errorf("token must remain is_used=false after rollback")
	}

	// Verify agent was NOT created
	if len(repo.agents) != 0 {
		t.Errorf("no agent record should exist after rollback")
	}
}

func TestAgentService_AuthenticateAgent(t *testing.T) {
	repo := newMockAgentRepository()
	hasher := &mockTokenHasher{}
	svc := agentService.NewAgentService(repo, hasher)
	ctx := context.Background()

	rawCred := "raw-agent-credential-secret-12345"
	credHash := hasher.HashToken(rawCred)

	now := time.Now()
	activeAgent := &agentModels.Agent{
		ID:        1,
		ServerID:  100,
		AgentID:   "agent-uuid-active",
		TokenHash: credHash,
		Version:   "0.1.0",
		Status:    agentModels.AgentStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	repo.agents[1] = activeAgent

	t.Run("Valid active credential succeeds", func(t *testing.T) {
		agent, err := svc.AuthenticateAgent(ctx, rawCred)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if agent.AgentID != "agent-uuid-active" {
			t.Errorf("expected agent_id agent-uuid-active, got %s", agent.AgentID)
		}
		if agent.ServerID != 100 {
			t.Errorf("expected server_id 100, got %d", agent.ServerID)
		}
	})

	t.Run("Empty credential returns unauthorized", func(t *testing.T) {
		_, err := svc.AuthenticateAgent(ctx, "   ")
		if !errors.Is(err, agentService.ErrUnauthorizedAgent) {
			t.Errorf("expected ErrUnauthorizedAgent, got %v", err)
		}
	})

	t.Run("Wrong credential returns unauthorized", func(t *testing.T) {
		_, err := svc.AuthenticateAgent(ctx, "wrong-credential-xyz")
		if !errors.Is(err, agentService.ErrUnauthorizedAgent) {
			t.Errorf("expected ErrUnauthorizedAgent, got %v", err)
		}
	})

	t.Run("Revoked agent returns unauthorized", func(t *testing.T) {
		revokedCred := "revoked-cred-xyz"
		revokedHash := hasher.HashToken(revokedCred)
		repo.agents[2] = &agentModels.Agent{
			ID:        2,
			ServerID:  200,
			AgentID:   "agent-uuid-revoked",
			TokenHash: revokedHash,
			Status:    agentModels.AgentStatusRevoked,
		}

		_, err := svc.AuthenticateAgent(ctx, revokedCred)
		if !errors.Is(err, agentService.ErrUnauthorizedAgent) {
			t.Errorf("expected ErrUnauthorizedAgent for revoked agent, got %v", err)
		}
	})

	t.Run("Inactive agent returns unauthorized", func(t *testing.T) {
		inactiveCred := "inactive-cred-xyz"
		inactiveHash := hasher.HashToken(inactiveCred)
		repo.agents[3] = &agentModels.Agent{
			ID:        3,
			ServerID:  300,
			AgentID:   "agent-uuid-inactive",
			TokenHash: inactiveHash,
			Status:    agentModels.AgentStatusInactive,
		}

		_, err := svc.AuthenticateAgent(ctx, inactiveCred)
		if !errors.Is(err, agentService.ErrUnauthorizedAgent) {
			t.Errorf("expected ErrUnauthorizedAgent for inactive agent, got %v", err)
		}
	})
}

func TestAgentService_Heartbeat(t *testing.T) {
	repo := newMockAgentRepository()
	hasher := &mockTokenHasher{}
	svc := agentService.NewAgentService(repo, hasher)
	ctx := context.Background()

	// Seed server 101 and agent
	repo.servers[101] = &serverModels.Server{
		ID:          101,
		UserID:      1,
		Name:        "Test VPS",
		AgentStatus: serverModels.StatusPending,
	}

	repo.agents[1] = &agentModels.Agent{
		ID:       1,
		ServerID: 101,
		AgentID:  "agent-heartbeat-101",
		Version:  "0.1.0",
		Status:   agentModels.AgentStatusActive,
	}

	t.Run("Valid heartbeat updates last_seen and transitions server ONLINE", func(t *testing.T) {
		req := agentDto.HeartbeatRequest{
			Timestamp:    "2026-09-29T06:10:00Z",
			AgentVersion: "0.1.1",
		}

		resp, err := svc.Heartbeat(ctx, "agent-heartbeat-101", 101, req)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		if resp.AgentID != "agent-heartbeat-101" {
			t.Errorf("expected agent_id agent-heartbeat-101, got %s", resp.AgentID)
		}
		if resp.ServerID != 101 {
			t.Errorf("expected server_id 101, got %d", resp.ServerID)
		}
		if resp.Status != serverModels.StatusOnline {
			t.Errorf("expected status ONLINE, got %s", resp.Status)
		}
		if resp.LastSeen == "" {
			t.Errorf("expected non-empty last_seen timestamp")
		}

		// Verify database objects were updated
		if repo.agents[1].LastSeen == nil {
			t.Fatal("expected agent.LastSeen to be updated")
		}
		if repo.agents[1].Version != "0.1.1" {
			t.Errorf("expected agent version 0.1.1, got %s", repo.agents[1].Version)
		}
		if repo.servers[101].LastSeen == nil {
			t.Fatal("expected server.LastSeen to be updated")
		}
		if repo.servers[101].AgentStatus != serverModels.StatusOnline {
			t.Errorf("expected server status ONLINE, got %s", repo.servers[101].AgentStatus)
		}
	})

	t.Run("Invalid timestamp format returns ErrInvalidTimestamp", func(t *testing.T) {
		req := agentDto.HeartbeatRequest{
			Timestamp: "invalid-timestamp-format",
		}
		_, err := svc.Heartbeat(ctx, "agent-heartbeat-101", 101, req)
		if !errors.Is(err, agentService.ErrInvalidTimestamp) {
			t.Errorf("expected ErrInvalidTimestamp, got %v", err)
		}
	})

	t.Run("Agent not found returns ErrAgentNotFound", func(t *testing.T) {
		req := agentDto.HeartbeatRequest{}
		_, err := svc.Heartbeat(ctx, "unknown-agent", 101, req)
		if !errors.Is(err, agentService.ErrAgentNotFound) {
			t.Errorf("expected ErrAgentNotFound, got %v", err)
		}
	})

	t.Run("Agent associated with wrong server returns ErrAgentNotFound", func(t *testing.T) {
		req := agentDto.HeartbeatRequest{}
		_, err := svc.Heartbeat(ctx, "agent-heartbeat-101", 999, req)
		if !errors.Is(err, agentService.ErrAgentNotFound) {
			t.Errorf("expected ErrAgentNotFound, got %v", err)
		}
	})

	t.Run("Inactive/revoked agent returns ErrAgentInactive", func(t *testing.T) {
		repo.agents[2] = &agentModels.Agent{
			ID:       2,
			ServerID: 102,
			AgentID:  "agent-revoked-102",
			Status:   agentModels.AgentStatusRevoked,
		}
		repo.servers[102] = &serverModels.Server{ID: 102, UserID: 1}

		req := agentDto.HeartbeatRequest{}
		_, err := svc.Heartbeat(ctx, "agent-revoked-102", 102, req)
		if !errors.Is(err, agentService.ErrAgentInactive) {
			t.Errorf("expected ErrAgentInactive, got %v", err)
		}
	})

	t.Run("Server not found returns ErrServerNotFound", func(t *testing.T) {
		repo.agents[3] = &agentModels.Agent{
			ID:       3,
			ServerID: 9999,
			AgentID:  "agent-orphaned",
			Status:   agentModels.AgentStatusActive,
		}
		req := agentDto.HeartbeatRequest{}
		_, err := svc.Heartbeat(ctx, "agent-orphaned", 9999, req)
		if !errors.Is(err, agentService.ErrServerNotFound) {
			t.Errorf("expected ErrServerNotFound, got %v", err)
		}
	})
}

func TestAgentService_DetectOfflineAgents(t *testing.T) {
	repo := newMockAgentRepository()
	hasher := &mockTokenHasher{}
	svc := agentService.NewAgentService(repo, hasher)
	ctx := context.Background()

	now := time.Now().UTC()
	recentTime := now.Add(-30 * time.Second)
	staleTime := now.Add(-100 * time.Second)

	// Server 1: ONLINE, recent -> stays ONLINE
	repo.servers[1] = &serverModels.Server{
		ID:          1,
		UserID:      1,
		AgentStatus: serverModels.StatusOnline,
		LastSeen:    &recentTime,
	}

	// Server 2: ONLINE, stale (>90s) -> transitions to OFFLINE
	repo.servers[2] = &serverModels.Server{
		ID:          2,
		UserID:      1,
		AgentStatus: serverModels.StatusOnline,
		LastSeen:    &staleTime,
	}

	// Server 3: OFFLINE, stale -> stays OFFLINE without repeated update
	repo.servers[3] = &serverModels.Server{
		ID:          3,
		UserID:      1,
		AgentStatus: serverModels.StatusOffline,
		LastSeen:    &staleTime,
	}

	// Server 4: Soft-deleted server with stale time -> not modified
	repo.servers[4] = &serverModels.Server{
		ID:          4,
		UserID:      1,
		AgentStatus: serverModels.StatusOnline,
		LastSeen:    &staleTime,
		DeletedAt:   gorm.DeletedAt{Time: now, Valid: true},
	}

	// Run detection with 90s threshold
	affected, err := svc.DetectOfflineAgents(ctx, 90*time.Second)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if affected != 1 {
		t.Errorf("expected exactly 1 server marked OFFLINE, got %d", affected)
	}

	if repo.servers[1].AgentStatus != serverModels.StatusOnline {
		t.Errorf("server 1 should remain ONLINE, got %s", repo.servers[1].AgentStatus)
	}
	if repo.servers[2].AgentStatus != serverModels.StatusOffline {
		t.Errorf("server 2 should become OFFLINE, got %s", repo.servers[2].AgentStatus)
	}
	if repo.servers[3].AgentStatus != serverModels.StatusOffline {
		t.Errorf("server 3 should remain OFFLINE, got %s", repo.servers[3].AgentStatus)
	}
	if repo.servers[4].AgentStatus != serverModels.StatusOnline {
		t.Errorf("deleted server 4 should not have been updated, got %s", repo.servers[4].AgentStatus)
	}

	// Run again immediately - verify 0 rows affected (no repeated update)
	affected2, err := svc.DetectOfflineAgents(ctx, 90*time.Second)
	if err != nil {
		t.Fatalf("expected nil error on second run, got %v", err)
	}
	if affected2 != 0 {
		t.Errorf("expected 0 servers affected on second run, got %d", affected2)
	}
}
