package repository

import (
	"context"

	"gorm.io/gorm"
	"vpsmonitoring-backend/internal/application/models"
)

type ApplicationRepository interface {
	Create(ctx context.Context, app *models.Application) error
	GetByID(ctx context.Context, id int64) (*models.Application, error)
	ListByServerID(ctx context.Context, serverID int64) ([]models.Application, error)
	ListEnabledByServerID(ctx context.Context, serverID int64) ([]models.Application, error)
	Update(ctx context.Context, app *models.Application) error
	Delete(ctx context.Context, id int64) error
	CountEnabledByServerID(ctx context.Context, serverID int64) (int64, error)
	GetByServerAndIdentity(ctx context.Context, serverID int64, matchType, matchValue string) (*models.Application, error)
}

type applicationRepository struct {
	db *gorm.DB
}

func NewApplicationRepository(db *gorm.DB) ApplicationRepository {
	return &applicationRepository{db: db}
}

func (r *applicationRepository) Create(ctx context.Context, app *models.Application) error {
	return r.db.WithContext(ctx).Create(app).Error
}

func (r *applicationRepository) GetByID(ctx context.Context, id int64) (*models.Application, error) {
	var app models.Application
	if err := r.db.WithContext(ctx).First(&app, id).Error; err != nil {
		return nil, err
	}
	return &app, nil
}

func (r *applicationRepository) ListByServerID(ctx context.Context, serverID int64) ([]models.Application, error) {
	var apps []models.Application
	if err := r.db.WithContext(ctx).Where("server_id = ?", serverID).Find(&apps).Error; err != nil {
		return nil, err
	}
	return apps, nil
}

func (r *applicationRepository) ListEnabledByServerID(ctx context.Context, serverID int64) ([]models.Application, error) {
	var apps []models.Application
	if err := r.db.WithContext(ctx).Where("server_id = ? AND is_enabled = ?", serverID, true).Find(&apps).Error; err != nil {
		return nil, err
	}
	return apps, nil
}

func (r *applicationRepository) Update(ctx context.Context, app *models.Application) error {
	return r.db.WithContext(ctx).Save(app).Error
}

func (r *applicationRepository) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&models.Application{}, id).Error
}

func (r *applicationRepository) CountEnabledByServerID(ctx context.Context, serverID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.Application{}).
		Where("server_id = ? AND is_enabled = ?", serverID, true).Count(&count).Error
	return count, err
}

func (r *applicationRepository) GetByServerAndIdentity(ctx context.Context, serverID int64, matchType, matchValue string) (*models.Application, error) {
	var app models.Application
	err := r.db.WithContext(ctx).
		Where("server_id = ? AND match_type = ? AND match_value = ?", serverID, matchType, matchValue).
		First(&app).Error
	if err != nil {
		return nil, err
	}
	return &app, nil
}
