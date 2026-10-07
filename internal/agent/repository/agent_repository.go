package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"vpsmonitoring-backend/internal/agent/models"
	tokenModels "vpsmonitoring-backend/internal/installation_token/models"
	serverModels "vpsmonitoring-backend/internal/server/models"
)

var (
	ErrTokenNotFound       = errors.New("installation token not found")
	ErrTokenExpired        = errors.New("installation token has expired")
	ErrTokenAlreadyUsed    = errors.New("installation token has already been used")
	ErrServerNotFound      = errors.New("server not found or has been deleted")
	ErrServerAlreadyActive = errors.New("server already has an active registered agent")
	ErrAgentNotFound       = errors.New("agent not found")
	ErrAgentInactive       = errors.New("agent is revoked or inactive")
)

// RegistrationParams holds the parameters for atomic agent registration transaction
type RegistrationParams struct {
	TokenHash         string
	AgentID           string
	CredentialHash    string
	AgentVersion      string
	Hostname          string
	IPAddress         string
	OS                string
	Architecture      string
	ServerAgentStatus string // "ONLINE"
}

// AgentRepository defines persistence methods for Agent entities
type AgentRepository interface {
	RegisterAgentTx(ctx context.Context, params RegistrationParams) (*models.Agent, int64, error)
	GetByAgentID(ctx context.Context, agentID string) (*models.Agent, error)
	GetByServerID(ctx context.Context, serverID int64) (*models.Agent, error)
	GetByCredentialHash(ctx context.Context, credentialHash string) (*models.Agent, error)
	UpdateHeartbeat(ctx context.Context, agentID string, serverID int64, timestamp time.Time, agentVersion string) error
	MarkStaleServersOffline(ctx context.Context, threshold time.Time) (int64, error)
	AutoMigrate() error
}

type gormAgentRepository struct {
	db *gorm.DB
}

// NewAgentRepository initializes an AgentRepository instance
func NewAgentRepository(db *gorm.DB) AgentRepository {
	return &gormAgentRepository{db: db}
}

// RegisterAgentTx executes agent creation, server update, and token consumption within a single database transaction
func (r *gormAgentRepository) RegisterAgentTx(ctx context.Context, params RegistrationParams) (*models.Agent, int64, error) {
	var createdAgent models.Agent
	var serverID int64

	now := time.Now()

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Fetch & lock the installation token
		var tok tokenModels.InstallationToken
		if err := tx.Where("token_hash = ?", params.TokenHash).First(&tok).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTokenNotFound
			}
			return err
		}

		if tok.IsUsed {
			return ErrTokenAlreadyUsed
		}

		if tok.IsExpired() {
			return ErrTokenExpired
		}

		serverID = tok.ServerID

		// 2. Fetch & verify associated server (must exist and not be soft-deleted)
		var srv serverModels.Server
		if err := tx.Where("id = ? AND deleted_at IS NULL", serverID).First(&srv).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrServerNotFound
			}
			return err
		}

		// 3. Create or update Agent record
		var existingAgent models.Agent
		agentExists := false
		if err := tx.Where("server_id = ?", serverID).First(&existingAgent).Error; err == nil {
			agentExists = true
		}

		if agentExists {
			// Update existing agent record for this server
			existingAgent.AgentID = params.AgentID
			existingAgent.TokenHash = params.CredentialHash
			existingAgent.Version = params.AgentVersion
			existingAgent.Status = models.AgentStatusActive
			existingAgent.UpdatedAt = now
			if err := tx.Save(&existingAgent).Error; err != nil {
				return err
			}
			createdAgent = existingAgent
		} else {
			// Create brand new agent record
			newAgent := models.Agent{
				ServerID:  serverID,
				AgentID:   params.AgentID,
				TokenHash: params.CredentialHash,
				Version:   params.AgentVersion,
				Status:    models.AgentStatusActive,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := tx.Create(&newAgent).Error; err != nil {
				return err
			}
			createdAgent = newAgent
		}

		// 4. Update Server with agent identity, system specs, and status transition (PENDING -> ONLINE)
		status := params.ServerAgentStatus
		if status == "" {
			status = serverModels.StatusOnline
		}

		serverUpdates := map[string]any{
			"hostname":     params.Hostname,
			"ip_address":   params.IPAddress,
			"os":           params.OS,
			"architecture": params.Architecture,
			"agent_id":     params.AgentID,
			"agent_status": status,
			"updated_at":   now,
		}

		if err := tx.Model(&serverModels.Server{}).Where("id = ?", serverID).Updates(serverUpdates).Error; err != nil {
			return err
		}

		// 5. Consume installation token (single-use enforcement)
		tokenUpdates := map[string]any{
			"is_used": true,
			"used_at": now,
		}
		if err := tx.Model(&tokenModels.InstallationToken{}).Where("id = ? AND is_used = ?", tok.ID, false).Updates(tokenUpdates).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, 0, err
	}

	return &createdAgent, serverID, nil
}

func (r *gormAgentRepository) GetByAgentID(ctx context.Context, agentID string) (*models.Agent, error) {
	var agent models.Agent
	result := r.db.WithContext(ctx).Where("agent_id = ?", agentID).First(&agent)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &agent, nil
}

func (r *gormAgentRepository) GetByServerID(ctx context.Context, serverID int64) (*models.Agent, error) {
	var agent models.Agent
	result := r.db.WithContext(ctx).Where("server_id = ?", serverID).First(&agent)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &agent, nil
}

func (r *gormAgentRepository) GetByCredentialHash(ctx context.Context, credentialHash string) (*models.Agent, error) {
	var agent models.Agent
	result := r.db.WithContext(ctx).Where("token_hash = ?", credentialHash).First(&agent)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &agent, nil
}

// UpdateHeartbeat atomically updates agent.last_seen and server.last_seen/agent_status in a transaction
func (r *gormAgentRepository) UpdateHeartbeat(ctx context.Context, agentID string, serverID int64, timestamp time.Time, agentVersion string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Verify agent exists, matches serverID, and is active
		var agent models.Agent
		if err := tx.Where("agent_id = ?", agentID).First(&agent).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAgentNotFound
			}
			return err
		}

		if agent.ServerID != serverID {
			return ErrAgentNotFound
		}

		if agent.Status != models.AgentStatusActive {
			return ErrAgentInactive
		}

		// 2. Verify server exists and is not soft-deleted
		var server serverModels.Server
		if err := tx.Where("id = ?", serverID).First(&server).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrServerNotFound
			}
			return err
		}

		// 3. Update agent last_seen (and version if provided)
		agentUpdates := map[string]interface{}{
			"last_seen":  timestamp,
			"updated_at": timestamp,
		}
		if agentVersion != "" {
			agentUpdates["version"] = agentVersion
		}

		if err := tx.Model(&models.Agent{}).
			Where("id = ?", agent.ID).
			Updates(agentUpdates).Error; err != nil {
			return err
		}

		// 4. Update server last_seen and keep/set agent_status = ONLINE
		serverUpdates := map[string]interface{}{
			"last_seen":    timestamp,
			"agent_status": serverModels.StatusOnline,
			"updated_at":   timestamp,
		}

		if err := tx.Model(&serverModels.Server{}).
			Where("id = ?", server.ID).
			Updates(serverUpdates).Error; err != nil {
			return err
		}

		return nil
	})
}

// MarkStaleServersOffline transitions servers with agent_status = 'ONLINE' and last_seen < threshold to 'OFFLINE'.
// It operates directly at database level, skips already OFFLINE servers, and respects soft deletion.
func (r *gormAgentRepository) MarkStaleServersOffline(ctx context.Context, threshold time.Time) (int64, error) {
	result := r.db.WithContext(ctx).
		Model(&serverModels.Server{}).
		Where("agent_status = ? AND last_seen < ? AND deleted_at IS NULL", serverModels.StatusOnline, threshold).
		Updates(map[string]interface{}{
			"agent_status": serverModels.StatusOffline,
			"updated_at":   time.Now().UTC(),
		})

	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// AutoMigrate creates the agents table if it does not exist.
func (r *gormAgentRepository) AutoMigrate() error {
	return r.db.AutoMigrate(&models.Agent{})
}
