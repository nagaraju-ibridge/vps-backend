package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"vpsmonitoring-backend/internal/installation_token/models"
)

// InstallationTokenRepository defines database persistence operations for installation tokens
type InstallationTokenRepository interface {
	Create(ctx context.Context, token *models.InstallationToken) error
	GetByHash(ctx context.Context, tokenHash string) (*models.InstallationToken, error)
	GetActiveByServerID(ctx context.Context, serverID int64) (*models.InstallationToken, error)
	MarkUsed(ctx context.Context, id int64) error
	InvalidateExistingForServer(ctx context.Context, serverID int64) error
	AutoMigrate() error
}

type gormInstallationTokenRepository struct {
	db *gorm.DB
}

// NewInstallationTokenRepository initializes a new GORM-backed installation token repository
func NewInstallationTokenRepository(db *gorm.DB) InstallationTokenRepository {
	return &gormInstallationTokenRepository{db: db}
}

func (r *gormInstallationTokenRepository) Create(ctx context.Context, token *models.InstallationToken) error {
	return r.db.WithContext(ctx).Create(token).Error
}

func (r *gormInstallationTokenRepository) GetByHash(ctx context.Context, tokenHash string) (*models.InstallationToken, error) {
	var token models.InstallationToken
	result := r.db.WithContext(ctx).Where("token_hash = ?", tokenHash).First(&token)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &token, nil
}

func (r *gormInstallationTokenRepository) GetActiveByServerID(ctx context.Context, serverID int64) (*models.InstallationToken, error) {
	var token models.InstallationToken
	now := time.Now()
	result := r.db.WithContext(ctx).
		Where("server_id = ? AND is_used = ? AND expires_at > ?", serverID, false, now).
		Order("id desc").
		First(&token)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &token, nil
}

func (r *gormInstallationTokenRepository) MarkUsed(ctx context.Context, id int64) error {
	now := time.Now()
	result := r.db.WithContext(ctx).
		Model(&models.InstallationToken{}).
		Where("id = ? AND is_used = ?", id, false).
		Updates(map[string]any{
			"is_used": true,
			"used_at": now,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("token not found or already used")
	}
	return nil
}

func (r *gormInstallationTokenRepository) InvalidateExistingForServer(ctx context.Context, serverID int64) error {
	now := time.Now()
	return r.db.WithContext(ctx).
		Model(&models.InstallationToken{}).
		Where("server_id = ? AND is_used = ? AND expires_at > ?", serverID, false, now).
		Update("expires_at", now).Error
}

// AutoMigrate creates the installation_tokens table if it does not exist.
func (r *gormInstallationTokenRepository) AutoMigrate() error {
	return r.db.AutoMigrate(&models.InstallationToken{})
}
