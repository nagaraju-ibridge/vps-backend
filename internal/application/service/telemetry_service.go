package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"vpsmonitoring-backend/internal/application/dto"
	"vpsmonitoring-backend/internal/application/models"
	"vpsmonitoring-backend/internal/application/repository"
)

var (
	ErrInvalidTimestamp = errors.New("invalid timestamp")
	ErrValidation       = errors.New("validation failed")
)

type TelemetryService interface {
	IngestTelemetry(ctx context.Context, serverID int64, batch dto.ApplicationTelemetryBatch) error
}

type telemetryService struct {
	telemetryRepo repository.TelemetryRepository
	appRepo       repository.ApplicationRepository
}

func NewTelemetryService(telemetryRepo repository.TelemetryRepository, appRepo repository.ApplicationRepository) TelemetryService {
	return &telemetryService{
		telemetryRepo: telemetryRepo,
		appRepo:       appRepo,
	}
}

func (s *telemetryService) IngestTelemetry(ctx context.Context, serverID int64, batch dto.ApplicationTelemetryBatch) error {
	now := time.Now().UTC()

	// Validate overall timestamp
	if batch.CollectedAt.IsZero() || batch.CollectedAt.After(now.Add(5*time.Minute)) || batch.CollectedAt.Before(now.Add(-24*time.Hour)) {
		return ErrInvalidTimestamp
	}

	for _, entry := range batch.Applications {
		// 1. Verify Ownership
		app, err := s.appRepo.GetByID(ctx, entry.ApplicationID)
		if err != nil {
			// Skip or fail batch? We will skip invalid entries to not block others
			continue
		}
		if app.ServerID != serverID {
			// Cross-server pollution attempt
			continue
		}
		if !app.IsEnabled {
			continue
		}

		// 2. Validate bounds
		if entry.CPUPercent != nil && (*entry.CPUPercent < 0 || *entry.CPUPercent > 100000) {
			continue
		}

		status := CalculateHealthStatus(entry)

		metric := &models.ApplicationMetric{
			ApplicationID:        app.ID,
			CollectedAt:          batch.CollectedAt,
			Status:               status,
			ProcessMatched:       entry.ProcessMatched,
			ProcessCount:         entry.ProcessCount,
			PrimaryPID:           entry.PrimaryPID,
			PrimaryStartTime:     entry.PrimaryStartTime,
			CPUPercent:           entry.CPUPercent,
			MemoryBytes:          entry.MemoryBytes,
			PortConfigured:       entry.PortConfigured,
			PortListening:        entry.PortListening,
			HTTPConfigured:       entry.HTTPConfigured,
			HTTPAvailable:        entry.HTTPAvailable,
			HTTPStatusCode:       entry.HTTPStatusCode,
			HTTPLatencyMs:        entry.HTTPLatencyMs,
			HTTPErrorClass:       entry.HTTPErrorClass,
			LogConfigured:        entry.LogConfigured,
			LogAvailable:         entry.LogAvailable,
			TotalRequests:        entry.TotalRequests,
			RequestsPerSecond:    entry.RequestsPerSecond,
			Status2xx:            entry.Status2xx,
			Status3xx:            entry.Status3xx,
			Status4xx:            entry.Status4xx,
			Status5xx:            entry.Status5xx,
			StatusOther:          entry.StatusOther,
			ActiveResponseCount:  entry.ActiveResponseCount,
			ActiveResponseAvgMs:  entry.ActiveResponseAvgMs,
			PassiveResponseCount: entry.PassiveResponseCount,
			PassiveResponseAvgMs: entry.PassiveResponseAvgMs,
		}

		var events []models.ApplicationEvent
		for _, ev := range entry.LifecycleEvents {
			// Validate event time
			if ev.EventTime.IsZero() || ev.EventTime.After(now.Add(5*time.Minute)) || ev.EventTime.Before(now.Add(-24*time.Hour)) {
				continue
			}

			// Bound details
			details := []byte(ev.Details)
			if len(details) > 1024 {
				continue
			}

			events = append(events, models.ApplicationEvent{
				ApplicationID: app.ID,
				EventType:     ev.EventType,
				EventTime:     ev.EventTime,
				OldPID:        ev.OldPID,
				NewPID:        ev.NewPID,
				Details:       json.RawMessage(details),
			})
		}

		// Persist for this app
		_ = s.telemetryRepo.PersistTelemetry(ctx, metric, events)
	}

	return nil
}
