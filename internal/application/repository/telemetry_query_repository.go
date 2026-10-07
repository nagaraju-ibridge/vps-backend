package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
	"vpsmonitoring-backend/internal/application/models"
)

// TelemetryQueryRepository provides read-only queries for application telemetry.
type TelemetryQueryRepository interface {
	GetLatestMetric(ctx context.Context, applicationID int64) (*models.ApplicationMetric, error)
	GetMetricHistory(ctx context.Context, applicationID int64, start, end time.Time, limit int) ([]models.ApplicationMetric, error)
	GetEvents(ctx context.Context, applicationID int64, limit int) ([]models.ApplicationEvent, error)
}

type telemetryQueryRepository struct {
	db *gorm.DB
}

func NewTelemetryQueryRepository(db *gorm.DB) TelemetryQueryRepository {
	return &telemetryQueryRepository{db: db}
}

// GetLatestMetric returns the single most recent metric for the given application.
// Returns gorm.ErrRecordNotFound if no rows exist.
func (r *telemetryQueryRepository) GetLatestMetric(ctx context.Context, applicationID int64) (*models.ApplicationMetric, error) {
	var m models.ApplicationMetric
	err := r.db.WithContext(ctx).
		Where("application_id = ?", applicationID).
		Order("collected_at DESC").
		Limit(1).
		First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// GetMetricHistory returns metrics for the given application ordered by collected_at ASC.
// The caller is responsible for validating start, end, and limit beforehand.
func (r *telemetryQueryRepository) GetMetricHistory(ctx context.Context, applicationID int64, start, end time.Time, limit int) ([]models.ApplicationMetric, error) {
	var metrics []models.ApplicationMetric
	err := r.db.WithContext(ctx).
		Where("application_id = ? AND collected_at >= ? AND collected_at <= ?", applicationID, start, end).
		Order("collected_at ASC").
		Limit(limit).
		Find(&metrics).Error
	if err != nil {
		return nil, err
	}
	return metrics, nil
}

// GetEvents returns lifecycle events for the given application ordered by event_time DESC.
// The caller is responsible for validating limit beforehand.
func (r *telemetryQueryRepository) GetEvents(ctx context.Context, applicationID int64, limit int) ([]models.ApplicationEvent, error) {
	var events []models.ApplicationEvent
	err := r.db.WithContext(ctx).
		Where("application_id = ?", applicationID).
		Order("event_time DESC").
		Limit(limit).
		Find(&events).Error
	if err != nil {
		return nil, err
	}
	return events, nil
}
