package service_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"vpsmonitoring-backend/internal/installation_token/models"
	"vpsmonitoring-backend/internal/installation_token/repository"
	"vpsmonitoring-backend/internal/installation_token/service"
	serverModels "vpsmonitoring-backend/internal/server/models"
	serverRepository "vpsmonitoring-backend/internal/server/repository"
)

// mockTokenRepository implements repository.InstallationTokenRepository in-memory
type mockTokenRepository struct {
	tokens map[int64]*models.InstallationToken
	nextID int64
}

func newMockTokenRepository() *mockTokenRepository {
	return &mockTokenRepository{
		tokens: make(map[int64]*models.InstallationToken),
		nextID: 1,
	}
}

func (m *mockTokenRepository) Create(ctx context.Context, token *models.InstallationToken) error {
	token.ID = m.nextID
	m.nextID++
	token.CreatedAt = time.Now()
	saved := *token
	m.tokens[token.ID] = &saved
	return nil
}

func (m *mockTokenRepository) GetByHash(ctx context.Context, tokenHash string) (*models.InstallationToken, error) {
	for _, tok := range m.tokens {
		if tok.TokenHash == tokenHash {
			cp := *tok
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockTokenRepository) GetActiveByServerID(ctx context.Context, serverID int64) (*models.InstallationToken, error) {
	now := time.Now()
	var latest *models.InstallationToken
	for _, tok := range m.tokens {
		if tok.ServerID == serverID && !tok.IsUsed && tok.ExpiresAt.After(now) {
			if latest == nil || tok.ID > latest.ID {
				latest = tok
			}
		}
	}
	if latest != nil {
		cp := *latest
		return &cp, nil
	}
	return nil, nil
}

func (m *mockTokenRepository) MarkUsed(ctx context.Context, id int64) error {
	tok, exists := m.tokens[id]
	if !exists || tok.IsUsed {
		return errors.New("token not found or already used")
	}
	now := time.Now()
	tok.IsUsed = true
	tok.UsedAt = &now
	return nil
}

func (m *mockTokenRepository) InvalidateExistingForServer(ctx context.Context, serverID int64) error {
	now := time.Now()
	for _, tok := range m.tokens {
		if tok.ServerID == serverID && !tok.IsUsed && tok.ExpiresAt.After(now) {
			tok.ExpiresAt = now
		}
	}
	return nil
}

func (m *mockTokenRepository) AutoMigrate() error {
	return nil
}

var _ repository.InstallationTokenRepository = (*mockTokenRepository)(nil)

// mockServerRepository implements serverRepository.ServerRepository in-memory
type mockServerRepository struct {
	servers map[int64]*serverModels.Server
}

func newMockServerRepository() *mockServerRepository {
	return &mockServerRepository{
		servers: make(map[int64]*serverModels.Server),
	}
}

func (m *mockServerRepository) AutoMigrate() error { return nil }
func (m *mockServerRepository) Create(ctx context.Context, server *serverModels.Server) error {
	m.servers[server.ID] = server
	return nil
}
func (m *mockServerRepository) GetByID(ctx context.Context, id int64) (*serverModels.Server, error) {
	return m.servers[id], nil
}
func (m *mockServerRepository) GetByIDAndUserID(ctx context.Context, id int64, userID int64) (*serverModels.Server, error) {
	srv, exists := m.servers[id]
	if !exists || srv.UserID != userID {
		return nil, nil
	}
	return srv, nil
}
func (m *mockServerRepository) GetByUserID(ctx context.Context, userID int64) ([]serverModels.Server, error) {
	return nil, nil
}
func (m *mockServerRepository) Update(ctx context.Context, server *serverModels.Server) error {
	return nil
}
func (m *mockServerRepository) Delete(ctx context.Context, id int64, userID int64) error {
	return nil
}

var _ serverRepository.ServerRepository = (*mockServerRepository)(nil)

func TestInstallationTokenService_GenerateAndHashToken(t *testing.T) {
	svc := service.NewInstallationTokenService(newMockTokenRepository(), newMockServerRepository(), "")

	tok1, err := svc.GenerateToken()
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}
	tok2, _ := svc.GenerateToken()

	if len(tok1) != 64 {
		t.Errorf("expected 64 hex chars (32 bytes), got length %d", len(tok1))
	}
	if tok1 == tok2 {
		t.Errorf("tokens must be cryptographically random and unique")
	}

	// Test SHA-256 Hashing
	hash := svc.HashToken(tok1)
	expectedSum := sha256.Sum256([]byte(tok1))
	expectedHex := hex.EncodeToString(expectedSum[:])

	if hash != expectedHex {
		t.Errorf("expected hash %s, got %s", expectedHex, hash)
	}
}

func TestInstallationTokenService_CreateToken_OwnershipAndSecurity(t *testing.T) {
	tokenRepo := newMockTokenRepository()
	serverRepo := newMockServerRepository()
	svc := service.NewInstallationTokenService(tokenRepo, serverRepo, "https://get.vpspulse.dev/agent.sh")
	ctx := context.Background()

	// Seed Server 1 owned by User 10
	serverRepo.servers[1] = &serverModels.Server{
		ID:          1,
		UserID:      10,
		Name:        "Production VPS",
		AgentStatus: "PENDING",
	}

	// 1. User 20 tries to create token for User 10's server -> MUST FAIL
	_, err := svc.CreateInstallationToken(ctx, 1, 20)
	if !errors.Is(err, service.ErrServerNotFound) {
		t.Errorf("expected ErrServerNotFound when User 20 accesses User 10's server, got %v", err)
	}

	// 2. User 10 creates token for Server 1 -> MUST SUCCEED
	res, err := svc.CreateInstallationToken(ctx, 1, 10)
	if err != nil {
		t.Fatalf("unexpected error creating installation token: %v", err)
	}

	if res.ServerID != 1 {
		t.Errorf("expected server_id 1, got %d", res.ServerID)
	}
	if len(res.Token) != 64 {
		t.Errorf("expected 64-char raw token, got %s", res.Token)
	}
	if !strings.Contains(res.InstallCommand, res.Token) {
		t.Errorf("install command must contain raw token, got: %s", res.InstallCommand)
	}
	if !strings.HasPrefix(res.InstallCommand, "curl -sSL https://get.vpspulse.dev/agent.sh | bash -s -- --token ") {
		t.Errorf("unexpected install command format: %s", res.InstallCommand)
	}

	// 3. Verify Database Persistence: Only the SHA-256 hash was stored, NEVER the raw token
	storedHash := svc.HashToken(res.Token)
	tokenInDB, err := tokenRepo.GetByHash(ctx, storedHash)
	if err != nil || tokenInDB == nil {
		t.Fatalf("expected token hash to be found in database")
	}
	if tokenInDB.TokenHash != storedHash {
		t.Errorf("expected stored hash %s, got %s", storedHash, tokenInDB.TokenHash)
	}
	if tokenInDB.TokenHash == res.Token {
		t.Errorf("CRITICAL SECURITY FLAW: raw token matches stored hash in DB")
	}
}

func TestInstallationTokenService_GetTokenStatus(t *testing.T) {
	tokenRepo := newMockTokenRepository()
	serverRepo := newMockServerRepository()
	svc := service.NewInstallationTokenService(tokenRepo, serverRepo, "")
	ctx := context.Background()

	serverRepo.servers[1] = &serverModels.Server{ID: 1, UserID: 10, Name: "VPS 1"}

	// 1. Before token generation -> status is exists=false
	status, err := svc.GetTokenStatus(ctx, 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Exists {
		t.Errorf("expected exists=false before token created")
	}

	// 2. Generate token
	res, err := svc.CreateInstallationToken(ctx, 1, 10)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// 3. Status now exists=true, is_used=false, expires_at set
	statusAfter, err := svc.GetTokenStatus(ctx, 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !statusAfter.Exists {
		t.Errorf("expected exists=true after token created")
	}
	if statusAfter.IsUsed {
		t.Errorf("expected is_used=false")
	}
	if statusAfter.ExpiresAt == nil || statusAfter.ExpiresAt.Before(time.Now()) {
		t.Errorf("expected valid future expires_at timestamp")
	}

	// 4. Mark token as used
	hash := svc.HashToken(res.Token)
	tok, _ := tokenRepo.GetByHash(ctx, hash)
	_ = tokenRepo.MarkUsed(ctx, tok.ID)

	// 5. Status query after usage -> active token no longer exists
	statusAfterUsed, err := svc.GetTokenStatus(ctx, 1, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if statusAfterUsed.Exists {
		t.Errorf("expected active token exists=false after token was marked used")
	}
}

func TestInstallationTokenService_ValidateToken(t *testing.T) {
	tokenRepo := newMockTokenRepository()
	serverRepo := newMockServerRepository()
	svc := service.NewInstallationTokenService(tokenRepo, serverRepo, "")
	ctx := context.Background()

	serverRepo.servers[1] = &serverModels.Server{ID: 1, UserID: 10, Name: "VPS 1"}
	created, _ := svc.CreateInstallationToken(ctx, 1, 10)

	// 1. Valid raw token -> successful validation
	validated, err := svc.ValidateToken(ctx, created.Token)
	if err != nil || validated == nil {
		t.Fatalf("expected valid token to pass validation: %v", err)
	}
	if validated.ServerID != 1 {
		t.Errorf("expected server_id 1, got %d", validated.ServerID)
	}

	// 2. Bogus/invalid token -> ErrInvalidToken
	_, err = svc.ValidateToken(ctx, "nonexistent-token-value")
	if !errors.Is(err, service.ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}

	// 3. Expired token -> ErrTokenExpired
	expiredRaw, _ := svc.GenerateToken()
	expiredHash := svc.HashToken(expiredRaw)
	_ = tokenRepo.Create(ctx, &models.InstallationToken{
		ServerID:  1,
		TokenHash: expiredHash,
		ExpiresAt: time.Now().Add(-10 * time.Minute),
		IsUsed:    false,
	})

	_, err = svc.ValidateToken(ctx, expiredRaw)
	if !errors.Is(err, service.ErrTokenExpired) {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}

	// 4. Used token -> ErrTokenAlreadyUsed
	usedRaw, _ := svc.GenerateToken()
	usedHash := svc.HashToken(usedRaw)
	tokUsed := &models.InstallationToken{
		ServerID:  1,
		TokenHash: usedHash,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		IsUsed:    false,
	}
	_ = tokenRepo.Create(ctx, tokUsed)
	_ = tokenRepo.MarkUsed(ctx, tokUsed.ID)

	_, err = svc.ValidateToken(ctx, usedRaw)
	if !errors.Is(err, service.ErrTokenAlreadyUsed) {
		t.Errorf("expected ErrTokenAlreadyUsed, got %v", err)
	}
}
