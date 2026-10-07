package service

import (
	"context"
	"testing"
	"time"

	"vpsmonitoring-backend/internal/application/dto"
	"vpsmonitoring-backend/internal/application/models"
)

// Mock Application Repo
type mockAppRepo struct {
	app *models.Application
	err error
}

func (m *mockAppRepo) Create(ctx context.Context, app *models.Application) error { return nil }
func (m *mockAppRepo) GetByID(ctx context.Context, id int64) (*models.Application, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.app != nil && m.app.ID == id {
		return m.app, nil
	}
	return nil, ErrApplicationNotFound
}
func (m *mockAppRepo) ListByServerID(ctx context.Context, serverID int64) ([]models.Application, error) {
	return nil, nil
}
func (m *mockAppRepo) ListEnabledByServerID(ctx context.Context, serverID int64) ([]models.Application, error) {
	return nil, nil
}
func (m *mockAppRepo) Update(ctx context.Context, app *models.Application) error { return nil }
func (m *mockAppRepo) Delete(ctx context.Context, id int64) error                { return nil }
func (m *mockAppRepo) CountEnabledByServerID(ctx context.Context, serverID int64) (int64, error) {
	return 0, nil
}
func (m *mockAppRepo) GetByServerAndIdentity(ctx context.Context, serverID int64, matchType, matchValue string) (*models.Application, error) {
	return nil, nil
}

// Mock Telemetry Repo
type mockTelemetryRepo struct {
	persisted int
	metric    *models.ApplicationMetric
}

func (m *mockTelemetryRepo) AutoMigrate() error { return nil }
func (m *mockTelemetryRepo) PersistTelemetry(ctx context.Context, metric *models.ApplicationMetric, events []models.ApplicationEvent) error {
	m.persisted++
	metricCopy := *metric
	m.metric = &metricCopy
	return nil
}

func TestTelemetryService_IngestTelemetry(t *testing.T) {
	appRepo := &mockAppRepo{
		app: &models.Application{ID: 1, ServerID: 100, IsEnabled: true},
	}
	telRepo := &mockTelemetryRepo{}
	svc := NewTelemetryService(telRepo, appRepo)

	now := time.Now().UTC()

	// Valid ingestion
	err := svc.IngestTelemetry(context.Background(), 100, dto.ApplicationTelemetryBatch{
		CollectedAt: now,
		Applications: []dto.ApplicationTelemetryEntry{
			{ApplicationID: 1, ObservationState: "OK"},
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if telRepo.persisted != 1 {
		t.Errorf("expected 1 persistence")
	}

	// Wrong server ID -> should silently skip (no block of batch, but doesn't persist)
	telRepo.persisted = 0
	_ = svc.IngestTelemetry(context.Background(), 200, dto.ApplicationTelemetryBatch{
		CollectedAt: now,
		Applications: []dto.ApplicationTelemetryEntry{
			{ApplicationID: 1, ObservationState: "OK"},
		},
	})
	if telRepo.persisted != 0 {
		t.Errorf("expected 0 persistence due to ownership violation")
	}

	// Invalid future timestamp -> fails the whole batch
	telRepo.persisted = 0
	err = svc.IngestTelemetry(context.Background(), 100, dto.ApplicationTelemetryBatch{
		CollectedAt: now.Add(2 * time.Hour), // too far in future
	})
	if err != ErrInvalidTimestamp {
		t.Errorf("expected ErrInvalidTimestamp")
	}
}

func TestTelemetryService_PreservesUnavailablePassiveResponseTime(t *testing.T) {
	appRepo := &mockAppRepo{app: &models.Application{ID: 1, ServerID: 100, IsEnabled: true}}
	telRepo := &mockTelemetryRepo{}
	svc := NewTelemetryService(telRepo, appRepo)
	now := time.Now().UTC()
	requests := int64(5)
	status4xx := int64(5)

	err := svc.IngestTelemetry(context.Background(), 100, dto.ApplicationTelemetryBatch{
		CollectedAt: now,
		Applications: []dto.ApplicationTelemetryEntry{{
			ApplicationID:        1,
			ObservationState:     "MATCHED",
			LogConfigured:        true,
			TotalRequests:        &requests,
			Status4xx:            &status4xx,
			PassiveResponseCount: nil,
			PassiveResponseAvgMs: nil,
		}},
	})
	if err != nil {
		t.Fatalf("ingest telemetry: %v", err)
	}
	if telRepo.metric == nil {
		t.Fatal("expected metric to be persisted")
	}
	if telRepo.metric.TotalRequests == nil || *telRepo.metric.TotalRequests != 5 || telRepo.metric.Status4xx == nil || *telRepo.metric.Status4xx != 5 {
		t.Fatalf("passive request metrics were not preserved: %+v", telRepo.metric)
	}
	if telRepo.metric.PassiveResponseCount != nil || telRepo.metric.PassiveResponseAvgMs != nil {
		t.Fatalf("unavailable passive timing must remain nil: %+v", telRepo.metric)
	}
}
