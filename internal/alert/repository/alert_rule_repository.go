package repository

import (
	"context"

	"gorm.io/gorm"
	"vpsmonitoring-backend/internal/alert/models"
)

type AlertRuleRepository interface {
	AutoMigrate() error
	Create(context.Context, *models.AlertRule) error
	GetByIDAndUserID(context.Context, int64, int64) (*models.AlertRule, error)
	ListByUserID(context.Context, int64) ([]models.AlertRule, error)
	ListEnabled(context.Context) ([]models.AlertRule, error)
	Update(context.Context, *models.AlertRule) error
	Delete(context.Context, *models.AlertRule) error
}

type alertRuleRepository struct{ db *gorm.DB }

func NewAlertRuleRepository(db *gorm.DB) AlertRuleRepository { return &alertRuleRepository{db: db} }
func (r *alertRuleRepository) AutoMigrate() error            { return r.db.AutoMigrate(&models.AlertRule{}) }
func (r *alertRuleRepository) Create(ctx context.Context, rule *models.AlertRule) error {
	return r.db.WithContext(ctx).Create(rule).Error
}
func (r *alertRuleRepository) GetByIDAndUserID(ctx context.Context, id, userID int64) (*models.AlertRule, error) {
	var rule models.AlertRule
	if err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&rule).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}
func (r *alertRuleRepository) ListByUserID(ctx context.Context, userID int64) ([]models.AlertRule, error) {
	var rules []models.AlertRule
	return rules, r.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at DESC").Find(&rules).Error
}
func (r *alertRuleRepository) ListEnabled(ctx context.Context) ([]models.AlertRule, error) {
	var rules []models.AlertRule
	return rules, r.db.WithContext(ctx).Where("is_enabled = ?", true).Order("id ASC").Limit(10000).Find(&rules).Error
}
func (r *alertRuleRepository) Update(ctx context.Context, rule *models.AlertRule) error {
	return r.db.WithContext(ctx).Save(rule).Error
}
func (r *alertRuleRepository) Delete(ctx context.Context, rule *models.AlertRule) error {
	return r.db.WithContext(ctx).Delete(rule).Error
}
