package repository

import (
	"errors"

	"gorm.io/gorm"
	"vpsmonitoring-backend/internal/health/models"
)

var (
	ErrConfigNotFound = errors.New("health configuration not found")
)

type HealthRepository interface {
	Create(config *models.HttpHealthConfig) error
	GetByID(id int64) (*models.HttpHealthConfig, error)
	ListByServerID(serverID int64) ([]models.HttpHealthConfig, error)
	ListActiveByServerID(serverID int64) ([]models.HttpHealthConfig, error)
	Delete(id int64) error
	CountByServerID(serverID int64) (int64, error)
	AutoMigrate() error
}

type healthRepository struct {
	db *gorm.DB
}

func NewHealthRepository(db *gorm.DB) HealthRepository {
	return &healthRepository{db: db}
}

func (r *healthRepository) Create(config *models.HttpHealthConfig) error {
	return r.db.Create(config).Error
}

func (r *healthRepository) GetByID(id int64) (*models.HttpHealthConfig, error) {
	var config models.HttpHealthConfig
	err := r.db.Where("id = ?", id).First(&config).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrConfigNotFound
		}
		return nil, err
	}
	return &config, nil
}

func (r *healthRepository) ListByServerID(serverID int64) ([]models.HttpHealthConfig, error) {
	var configs []models.HttpHealthConfig
	err := r.db.Where("server_id = ?", serverID).Order("created_at asc").Find(&configs).Error
	return configs, err
}

func (r *healthRepository) ListActiveByServerID(serverID int64) ([]models.HttpHealthConfig, error) {
	var configs []models.HttpHealthConfig
	err := r.db.Where("server_id = ? AND is_active = ?", serverID, true).Order("created_at asc").Find(&configs).Error
	return configs, err
}

func (r *healthRepository) Delete(id int64) error {
	result := r.db.Delete(&models.HttpHealthConfig{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrConfigNotFound
	}
	return nil
}

func (r *healthRepository) CountByServerID(serverID int64) (int64, error) {
	var count int64
	err := r.db.Model(&models.HttpHealthConfig{}).Where("server_id = ?", serverID).Count(&count).Error
	return count, err
}

func (r *healthRepository) AutoMigrate() error {
	return r.db.AutoMigrate(&models.HttpHealthConfig{})
}
