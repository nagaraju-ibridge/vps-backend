package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"vpsmonitoring-backend/internal/installation_token/dto"
	"vpsmonitoring-backend/internal/installation_token/models"
	"vpsmonitoring-backend/internal/installation_token/repository"
	serverRepository "vpsmonitoring-backend/internal/server/repository"
)

var (
	ErrServerNotFound   = errors.New("server not found")
	ErrInvalidToken     = errors.New("invalid installation token")
	ErrTokenExpired     = errors.New("installation token has expired")
	ErrTokenAlreadyUsed = errors.New("installation token has already been used")
)

// InstallationTokenService defines business operations for token lifecycle management
type InstallationTokenService interface {
	GenerateToken() (string, error)
	HashToken(rawToken string) string
	CreateInstallationToken(ctx context.Context, serverID int64, userID int64) (*dto.CreateInstallationTokenResponse, error)
	GetTokenStatus(ctx context.Context, serverID int64, userID int64) (*dto.TokenStatusResponse, error)
	ValidateToken(ctx context.Context, rawToken string) (*models.InstallationToken, error)
}

type installationTokenService struct {
	tokenRepo        repository.InstallationTokenRepository
	serverRepo       serverRepository.ServerRepository
	installScriptURL string
}

// NewInstallationTokenService initializes an InstallationTokenService instance
func NewInstallationTokenService(
	tokenRepo repository.InstallationTokenRepository,
	serverRepo serverRepository.ServerRepository,
	installScriptURL string,
) InstallationTokenService {
	if installScriptURL == "" {
		installScriptURL = "https://get.vpspulse.dev/agent.sh"
	}
	return &installationTokenService{
		tokenRepo:        tokenRepo,
		serverRepo:       serverRepo,
		installScriptURL: installScriptURL,
	}
}

// GenerateToken generates a cryptographically secure 32-byte (256-bit) random token, hex-encoded (64 chars)
func (s *installationTokenService) GenerateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure random token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// HashToken computes the SHA-256 cryptographic hash of the raw token (64-char hex string)
func (s *installationTokenService) HashToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

// CreateInstallationToken generates a single-use token, persists its SHA-256 hash, and returns the raw install command
func (s *installationTokenService) CreateInstallationToken(ctx context.Context, serverID int64, userID int64) (*dto.CreateInstallationTokenResponse, error) {
	// 1. Verify server existence & caller ownership
	server, err := s.serverRepo.GetByIDAndUserID(ctx, serverID, userID)
	if err != nil {
		return nil, err
	}
	if server == nil {
		return nil, ErrServerNotFound
	}

	// 2. Invalidate any existing unused tokens for this server so only one active token remains
	_ = s.tokenRepo.InvalidateExistingForServer(ctx, serverID)

	// 3. Generate 32-byte cryptographically secure random token
	rawToken, err := s.GenerateToken()
	if err != nil {
		return nil, err
	}

	// 4. Compute SHA-256 hash of the raw token for secure database persistence
	tokenHash := s.HashToken(rawToken)

	// 5. Token expires in 1 hour
	expiresAt := time.Now().Add(1 * time.Hour)

	// 6. Store only the hash in PostgreSQL
	entity := models.InstallationToken{
		ServerID:  serverID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		IsUsed:    false,
	}

	if err := s.tokenRepo.Create(ctx, &entity); err != nil {
		return nil, fmt.Errorf("failed to persist installation token hash: %w", err)
	}

	// 7. Format the one-line agent install command
	installCommand := fmt.Sprintf("curl -sSL %s | bash -s -- --token %s", s.installScriptURL, rawToken)

	return &dto.CreateInstallationTokenResponse{
		ServerID:       serverID,
		Token:          rawToken,
		ExpiresAt:      expiresAt,
		InstallCommand: installCommand,
	}, nil
}

// GetTokenStatus returns metadata on whether an active token exists for the server (raw token/hash are never leaked)
func (s *installationTokenService) GetTokenStatus(ctx context.Context, serverID int64, userID int64) (*dto.TokenStatusResponse, error) {
	// 1. Verify server existence & caller ownership
	server, err := s.serverRepo.GetByIDAndUserID(ctx, serverID, userID)
	if err != nil {
		return nil, err
	}
	if server == nil {
		return nil, ErrServerNotFound
	}

	// 2. Query active unexpired, unused token
	token, err := s.tokenRepo.GetActiveByServerID(ctx, serverID)
	if err != nil {
		return nil, err
	}

	if token == nil {
		return &dto.TokenStatusResponse{
			Exists: false,
			IsUsed: false,
		}, nil
	}

	return &dto.TokenStatusResponse{
		Exists:    true,
		ExpiresAt: &token.ExpiresAt,
		IsUsed:    token.IsUsed,
	}, nil
}

// ValidateToken is used by the agent onboarding process (Phase 2C) to verify credentials
func (s *installationTokenService) ValidateToken(ctx context.Context, rawToken string) (*models.InstallationToken, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return nil, ErrInvalidToken
	}

	tokenHash := s.HashToken(rawToken)
	token, err := s.tokenRepo.GetByHash(ctx, tokenHash)
	if err != nil {
		return nil, err
	}
	if token == nil {
		return nil, ErrInvalidToken
	}

	if token.IsUsed {
		return nil, ErrTokenAlreadyUsed
	}

	if token.IsExpired() {
		return nil, ErrTokenExpired
	}

	return token, nil
}
