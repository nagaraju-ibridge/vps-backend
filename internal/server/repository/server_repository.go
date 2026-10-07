package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"vpsmonitoring-backend/internal/server/models"
)

// ServerRepository defines data access methods for the Server entity
type ServerRepository interface {
	Create(ctx context.Context, server *models.Server) error
	GetByID(ctx context.Context, id int64) (*models.Server, error)
	GetByIDAndUserID(ctx context.Context, id int64, userID int64) (*models.Server, error)
	GetByUserID(ctx context.Context, userID int64) ([]models.Server, error)
	Update(ctx context.Context, server *models.Server) error
	Delete(ctx context.Context, id int64, userID int64) error
	AutoMigrate() error
}

type gormServerRepository struct {
	db *gorm.DB
}

// NewServerRepository initializes a GORM-backed ServerRepository
func NewServerRepository(db *gorm.DB) ServerRepository {
	return &gormServerRepository{db: db}
}

// AutoMigrate migrates the servers table schema
func (r *gormServerRepository) AutoMigrate() error {
	return r.db.AutoMigrate(&models.Server{})
}

func (r *gormServerRepository) Create(ctx context.Context, server *models.Server) error {
	return r.db.WithContext(ctx).Create(server).Error
}

func (r *gormServerRepository) GetByID(ctx context.Context, id int64) (*models.Server, error) {
	var server models.Server
	result := r.db.WithContext(ctx).First(&server, id)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &server, nil
}

func (r *gormServerRepository) GetByIDAndUserID(ctx context.Context, id int64, userID int64) (*models.Server, error) {
	var server models.Server
	result := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&server)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &server, nil
}

func (r *gormServerRepository) GetByUserID(ctx context.Context, userID int64) ([]models.Server, error) {
	var servers []models.Server
	result := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("id desc").Find(&servers)
	if result.Error != nil {
		return nil, result.Error
	}
	return servers, nil
}

func (r *gormServerRepository) Update(ctx context.Context, server *models.Server) error {
	return r.db.WithContext(ctx).Save(server).Error
}

func (r *gormServerRepository) Delete(ctx context.Context, id int64, userID int64) error {
	// GORM soft-deletes records with deleted_at timestamp
	result := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).Delete(&models.Server{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
