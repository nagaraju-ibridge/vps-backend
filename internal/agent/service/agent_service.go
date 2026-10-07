package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"vpsmonitoring-backend/internal/agent/dto"
	"vpsmonitoring-backend/internal/agent/models"
	"vpsmonitoring-backend/internal/agent/repository"
	tokenService "vpsmonitoring-backend/internal/installation_token/service"
	serverModels "vpsmonitoring-backend/internal/server/models"
)

var (
	ErrMissingInstallationToken = errors.New("installation_token is required")
	ErrInvalidInstallationToken = errors.New("invalid installation token")
	ErrTokenExpired             = errors.New("installation token has expired")
	ErrTokenAlreadyUsed         = errors.New("installation token has already been used")
	ErrServerNotFound           = errors.New("server not found or has been deleted")
	ErrUnauthorizedAgent        = errors.New("unauthorized: invalid agent credential")
	ErrAgentNotFound            = errors.New("agent not found")
	ErrAgentInactive            = errors.New("agent is revoked or inactive")
	ErrInvalidTimestamp         = errors.New("invalid timestamp format, must be RFC3339")
)

// AgentService defines business logic operations for agent lifecycle
type AgentService interface {
	Register(ctx context.Context, req dto.RegisterAgentRequest) (*dto.RegisterAgentResponse, error)
	AuthenticateAgent(ctx context.Context, rawCredential string) (*models.Agent, error)
	Heartbeat(ctx context.Context, agentID string, serverID int64, req dto.HeartbeatRequest) (*dto.HeartbeatResponse, error)
	DetectOfflineAgents(ctx context.Context, offlineThreshold time.Duration) (int64, error)
}

type agentService struct {
	agentRepo    repository.AgentRepository
	tokenService tokenService.InstallationTokenService
}

// NewAgentService creates a new AgentService instance
func NewAgentService(
	agentRepo repository.AgentRepository,
	tokenService tokenService.InstallationTokenService,
) AgentService {
	return &agentService{
		agentRepo:    agentRepo,
		tokenService: tokenService,
	}
}

// Register processes agent onboarding using an installation token inside a single DB transaction
func (s *agentService) Register(ctx context.Context, req dto.RegisterAgentRequest) (*dto.RegisterAgentResponse, error) {
	// 1. Validate inputs
	rawToken := strings.TrimSpace(req.InstallationToken)
	if rawToken == "" {
		return nil, ErrMissingInstallationToken
	}

	hostname := strings.TrimSpace(req.Hostname)
	ipAddress := strings.TrimSpace(req.IPAddress)
	osName := strings.TrimSpace(req.OS)
	architecture := strings.TrimSpace(req.Architecture)
	agentVersion := strings.TrimSpace(req.AgentVersion)
	if agentVersion == "" {
		agentVersion = "0.1.0"
	}

	// 2. Hash raw installation token before querying (reusing Phase 2B token service logic)
	tokenHash := s.tokenService.HashToken(rawToken)

	// 3. Generate permanent UUID v4 for agent_id
	agentID := uuid.New().String()

	// 4. Generate cryptographically secure random 32-byte agent credential (64-char hex)
	credBytes := make([]byte, 32)
	if _, err := rand.Read(credBytes); err != nil {
		return nil, fmt.Errorf("failed to generate secure agent credential: %w", err)
	}
	rawCredential := hex.EncodeToString(credBytes)

	// 5. Compute SHA-256 hash of the credential for persistent storage (never store raw credential)
	credentialHash := s.tokenService.HashToken(rawCredential)

	// 6. Execute atomic database transaction
	params := repository.RegistrationParams{
		TokenHash:         tokenHash,
		AgentID:           agentID,
		CredentialHash:    credentialHash,
		AgentVersion:      agentVersion,
		Hostname:          hostname,
		IPAddress:         ipAddress,
		OS:                osName,
		Architecture:      architecture,
		ServerAgentStatus: serverModels.StatusOnline, // Transition server status PENDING -> ONLINE
	}

	agent, serverID, err := s.agentRepo.RegisterAgentTx(ctx, params)
	if err != nil {
		if errors.Is(err, repository.ErrTokenNotFound) {
			return nil, ErrInvalidInstallationToken
		}
		if errors.Is(err, repository.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		if errors.Is(err, repository.ErrTokenAlreadyUsed) {
			return nil, ErrTokenAlreadyUsed
		}
		if errors.Is(err, repository.ErrServerNotFound) {
			return nil, ErrServerNotFound
		}
		return nil, err
	}

	// 7. Deliver the raw credential once in the registration response
	return &dto.RegisterAgentResponse{
		AgentID:    agent.AgentID,
		ServerID:   serverID,
		Credential: rawCredential,
		Status:     models.AgentStatusActive,
	}, nil
}

// AuthenticateAgent validates an agent credential by hashing it with SHA-256 and checking the agent record
func (s *agentService) AuthenticateAgent(ctx context.Context, rawCredential string) (*models.Agent, error) {
	rawCredential = strings.TrimSpace(rawCredential)
	if rawCredential == "" {
		return nil, ErrUnauthorizedAgent
	}

	// 1. Hash the incoming raw credential with SHA-256 (matches registration hashing)
	credentialHash := s.tokenService.HashToken(rawCredential)

	// 2. Query agent by credential hash
	agent, err := s.agentRepo.GetByCredentialHash(ctx, credentialHash)
	if err != nil {
		return nil, err
	}
	if agent == nil {
		return nil, ErrUnauthorizedAgent
	}

	// 3. Reject revoked or inactive agents
	if agent.Status != models.AgentStatusActive {
		return nil, ErrUnauthorizedAgent
	}

	return agent, nil
}

// Heartbeat processes a periodic agent ping, recording the authoritative server time
func (s *agentService) Heartbeat(ctx context.Context, agentID string, serverID int64, req dto.HeartbeatRequest) (*dto.HeartbeatResponse, error) {
	// 1. Validate payload timestamp if provided (optional or RFC3339)
	reqTimestamp := strings.TrimSpace(req.Timestamp)
	if reqTimestamp != "" {
		if _, err := time.Parse(time.RFC3339, reqTimestamp); err != nil {
			return nil, ErrInvalidTimestamp
		}
	}

	agentVersion := strings.TrimSpace(req.AgentVersion)

	// 2. Authoritative backend UTC timestamp (never trust VPS clock for last_seen)
	now := time.Now().UTC()

	// 3. Atomically update agent.last_seen and server.last_seen, setting server status to ONLINE
	if err := s.agentRepo.UpdateHeartbeat(ctx, agentID, serverID, now, agentVersion); err != nil {
		if errors.Is(err, repository.ErrAgentNotFound) {
			return nil, ErrAgentNotFound
		}
		if errors.Is(err, repository.ErrAgentInactive) {
			return nil, ErrAgentInactive
		}
		if errors.Is(err, repository.ErrServerNotFound) {
			return nil, ErrServerNotFound
		}
		return nil, err
	}

	return &dto.HeartbeatResponse{
		AgentID:  agentID,
		ServerID: serverID,
		Status:   serverModels.StatusOnline,
		LastSeen: now.Format(time.RFC3339),
	}, nil
}

// DetectOfflineAgents checks for ONLINE servers whose last_seen exceeds offlineThreshold and marks them OFFLINE
func (s *agentService) DetectOfflineAgents(ctx context.Context, offlineThreshold time.Duration) (int64, error) {
	thresholdTime := time.Now().UTC().Add(-offlineThreshold)
	return s.agentRepo.MarkStaleServersOffline(ctx, thresholdTime)
}
