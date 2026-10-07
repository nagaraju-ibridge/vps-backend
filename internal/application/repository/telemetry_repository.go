package repository

import (
	"context"

	"gorm.io/gorm"
	"vpsmonitoring-backend/internal/application/models"
)

type TelemetryRepository interface {
	AutoMigrate() error
	PersistTelemetry(ctx context.Context, metric *models.ApplicationMetric, events []models.ApplicationEvent) error
}

type telemetryRepository struct {
	db *gorm.DB
}

func NewTelemetryRepository(db *gorm.DB) TelemetryRepository {
	return &telemetryRepository{db: db}
}

func (r *telemetryRepository) AutoMigrate() error {
	return r.db.AutoMigrate(
		&models.ApplicationMetric{},
		&models.ApplicationEvent{},
	)
}

func (r *telemetryRepository) PersistTelemetry(ctx context.Context, metric *models.ApplicationMetric, events []models.ApplicationEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Idempotency check: see if metric for this application_id and collected_at already exists
		var count int64
		if err := tx.Model(&models.ApplicationMetric{}).
			Where("application_id = ? AND collected_at = ?", metric.ApplicationID, metric.CollectedAt).
			Count(&count).Error; err != nil {
			return err
		}

		// Skip if already exists
		if count > 0 {
			return nil
		}

		if err := tx.Create(metric).Error; err != nil {
			return err
		}

		if len(events) > 0 {
			// Idempotency on events: since agent sends all events observed in this cycle,
			// we can rely on (application_id, event_type, event_time) to avoid dups.
			// The simplest way to handle batch idempotency is to just create them.
			// Because we skipped if the metric exists, we also skip the events that came with it.
			if err := tx.Create(&events).Error; err != nil {
				return err
			}
		}

		return nil
	})
}
