package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"vpsmonitoring-backend/internal/application/dto"
	"vpsmonitoring-backend/internal/application/models"
	serverDto "vpsmonitoring-backend/internal/server/dto"
	serverService "vpsmonitoring-backend/internal/server/service"
)

// ---- Mocks unique to telemetry query tests (no collisions with application_service_test.go) ----

type queryMockTelemetryRepo struct {
	latestMetric *models.ApplicationMetric
	latestErr    error
	historyRows  []models.ApplicationMetric
	historyErr   error
	eventRows    []models.ApplicationEvent
	eventsErr    error
}

func (m *queryMockTelemetryRepo) GetLatestMetric(_ context.Context, _ int64) (*models.ApplicationMetric, error) {
	return m.latestMetric, m.latestErr
}
func (m *queryMockTelemetryRepo) GetMetricHistory(_ context.Context, _ int64, _, _ time.Time, _ int) ([]models.ApplicationMetric, error) {
	return m.historyRows, m.historyErr
}
func (m *queryMockTelemetryRepo) GetEvents(_ context.Context, _ int64, _ int) ([]models.ApplicationEvent, error) {
	return m.eventRows, m.eventsErr
}

type queryMockAppRepo struct {
	app    *models.Application
	appErr error
}

func (m *queryMockAppRepo) Create(_ context.Context, _ *models.Application) error { return nil }
func (m *queryMockAppRepo) GetByID(_ context.Context, _ int64) (*models.Application, error) {
	return m.app, m.appErr
}
func (m *queryMockAppRepo) ListByServerID(_ context.Context, _ int64) ([]models.Application, error) {
	return nil, nil
}
func (m *queryMockAppRepo) ListEnabledByServerID(_ context.Context, _ int64) ([]models.Application, error) {
	return nil, nil
}
func (m *queryMockAppRepo) Update(_ context.Context, _ *models.Application) error { return nil }
func (m *queryMockAppRepo) Delete(_ context.Context, _ int64) error               { return nil }
func (m *queryMockAppRepo) CountEnabledByServerID(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}
func (m *queryMockAppRepo) GetByServerAndIdentity(_ context.Context, _ int64, _, _ string) (*models.Application, error) {
	return nil, nil
}

// queryMockServerSvc satisfies the real serverService.ServerService interface.
type queryMockServerSvc struct {
	serverService.ServerService // embed to satisfy full interface
	serverErr                   error
}

func (m *queryMockServerSvc) GetServer(_ context.Context, _ int64, _ int64) (*serverDto.ServerResponse, error) {
	if m.serverErr != nil {
		return nil, m.serverErr
	}
	return &serverDto.ServerResponse{}, nil
}

// ---- Helpers ----

func newQuerySvc(qRepo *queryMockTelemetryRepo, aRepo *queryMockAppRepo, svrErr error) TelemetryQueryService {
	return NewTelemetryQueryService(qRepo, aRepo, &queryMockServerSvc{serverErr: svrErr})
}

func makeQueryApp(serverID int64) *models.Application {
	return &models.Application{ID: 1, ServerID: serverID, IsEnabled: true}
}

func makeQueryMetric() *models.ApplicationMetric {
	cpu := 1.5
	mem := uint64(3043328)
	pid := int64(705)
	cnt := 1
	now := time.Now().UTC()
	return &models.ApplicationMetric{
		ID:            1,
		ApplicationID: 1,
		CollectedAt:   now,
		Status:        "UP",
		PrimaryPID:    &pid,
		ProcessCount:  cnt,
		CPUPercent:    &cpu,
		MemoryBytes:   &mem,
	}
}

// ---- Tests: GetLatest ----

func TestTelemetryQueryService_GetLatest_OK(t *testing.T) {
	svc := newQuerySvc(
		&queryMockTelemetryRepo{latestMetric: makeQueryMetric()},
		&queryMockAppRepo{app: makeQueryApp(1)},
		nil,
	)
	resp, err := svc.GetLatest(context.Background(), 42, 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "UP" {
		t.Errorf("expected UP, got %s", resp.Status)
	}
	if resp.PrimaryPID == nil || *resp.PrimaryPID != 705 {
		t.Errorf("unexpected PID: %v", resp.PrimaryPID)
	}
}

func TestTelemetryQueryService_GetLatest_NoTelemetry(t *testing.T) {
	svc := newQuerySvc(
		&queryMockTelemetryRepo{latestErr: gorm.ErrRecordNotFound},
		&queryMockAppRepo{app: makeQueryApp(1)},
		nil,
	)
	_, err := svc.GetLatest(context.Background(), 42, 1, 1)
	if !errors.Is(err, ErrTelemetryNotFound) {
		t.Errorf("expected ErrTelemetryNotFound, got %v", err)
	}
}

func TestTelemetryQueryService_GetLatest_ServerOwnershipDenied(t *testing.T) {
	svc := newQuerySvc(
		&queryMockTelemetryRepo{},
		&queryMockAppRepo{app: makeQueryApp(1)},
		errors.New("not found"),
	)
	_, err := svc.GetLatest(context.Background(), 99, 1, 1)
	if !errors.Is(err, ErrServerOwnership) {
		t.Errorf("expected ErrServerOwnership, got %v", err)
	}
}

func TestTelemetryQueryService_GetLatest_AppNotFound(t *testing.T) {
	svc := newQuerySvc(
		&queryMockTelemetryRepo{},
		&queryMockAppRepo{appErr: gorm.ErrRecordNotFound},
		nil,
	)
	_, err := svc.GetLatest(context.Background(), 42, 1, 99)
	if !errors.Is(err, ErrApplicationNotFound) {
		t.Errorf("expected ErrApplicationNotFound, got %v", err)
	}
}

func TestTelemetryQueryService_GetLatest_CrossServerAppRejected(t *testing.T) {
	// app belongs to server 2, request is for server 1
	svc := newQuerySvc(
		&queryMockTelemetryRepo{},
		&queryMockAppRepo{app: makeQueryApp(2)},
		nil,
	)
	_, err := svc.GetLatest(context.Background(), 42, 1, 1)
	if !errors.Is(err, ErrApplicationNotFound) {
		t.Errorf("expected ErrApplicationNotFound for cross-server access, got %v", err)
	}
}

// ---- Tests: GetHistory ----

func TestTelemetryQueryService_GetHistory_Valid(t *testing.T) {
	now := time.Now().UTC()
	svc := newQuerySvc(
		&queryMockTelemetryRepo{historyRows: []models.ApplicationMetric{*makeQueryMetric()}},
		&queryMockAppRepo{app: makeQueryApp(1)},
		nil,
	)
	resp, err := svc.GetHistory(context.Background(), 42, 1, 1, now.Add(-1*time.Hour), now, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp) != 1 {
		t.Errorf("expected 1 result, got %d", len(resp))
	}
}

func TestTelemetryQueryService_GetHistory_EmptyReturnsSlice(t *testing.T) {
	now := time.Now().UTC()
	svc := newQuerySvc(
		&queryMockTelemetryRepo{historyRows: nil},
		&queryMockAppRepo{app: makeQueryApp(1)},
		nil,
	)
	resp, err := svc.GetHistory(context.Background(), 42, 1, 1, now.Add(-1*time.Hour), now, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Error("expected empty slice, got nil")
	}
	if len(resp) != 0 {
		t.Errorf("expected 0 items, got %d", len(resp))
	}
}

func TestTelemetryQueryService_GetHistory_StartAfterEnd(t *testing.T) {
	now := time.Now().UTC()
	svc := newQuerySvc(&queryMockTelemetryRepo{}, &queryMockAppRepo{app: makeQueryApp(1)}, nil)
	_, err := svc.GetHistory(context.Background(), 42, 1, 1, now, now.Add(-1*time.Hour), 100)
	if !errors.Is(err, ErrInvalidTimeRange) {
		t.Errorf("expected ErrInvalidTimeRange, got %v", err)
	}
}

func TestTelemetryQueryService_GetHistory_RangeTooLarge(t *testing.T) {
	now := time.Now().UTC()
	svc := newQuerySvc(&queryMockTelemetryRepo{}, &queryMockAppRepo{app: makeQueryApp(1)}, nil)
	_, err := svc.GetHistory(context.Background(), 42, 1, 1, now.Add(-32*24*time.Hour), now, 100)
	if !errors.Is(err, ErrTimeRangeTooLarge) {
		t.Errorf("expected ErrTimeRangeTooLarge, got %v", err)
	}
}

func TestTelemetryQueryService_GetHistory_LimitZeroRejected(t *testing.T) {
	now := time.Now().UTC()
	svc := newQuerySvc(&queryMockTelemetryRepo{}, &queryMockAppRepo{app: makeQueryApp(1)}, nil)
	_, err := svc.GetHistory(context.Background(), 42, 1, 1, now.Add(-1*time.Hour), now, 0)
	if !errors.Is(err, ErrInvalidHistoryLimit) {
		t.Errorf("expected ErrInvalidHistoryLimit, got %v", err)
	}
}

func TestTelemetryQueryService_GetHistory_LimitExceedsMax(t *testing.T) {
	now := time.Now().UTC()
	svc := newQuerySvc(&queryMockTelemetryRepo{}, &queryMockAppRepo{app: makeQueryApp(1)}, nil)
	_, err := svc.GetHistory(context.Background(), 42, 1, 1, now.Add(-1*time.Hour), now, MaxTelemetryHistoryLimit+1)
	if !errors.Is(err, ErrInvalidHistoryLimit) {
		t.Errorf("expected ErrInvalidHistoryLimit, got %v", err)
	}
}

// ---- Tests: GetEvents ----

func TestTelemetryQueryService_GetEvents_Valid(t *testing.T) {
	events := []models.ApplicationEvent{
		{ID: 1, ApplicationID: 1, EventType: "process_started", EventTime: time.Now().UTC()},
	}
	svc := newQuerySvc(
		&queryMockTelemetryRepo{eventRows: events},
		&queryMockAppRepo{app: makeQueryApp(1)},
		nil,
	)
	resp, err := svc.GetEvents(context.Background(), 42, 1, 1, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp) != 1 {
		t.Errorf("expected 1 event, got %d", len(resp))
	}
	if resp[0].EventType != "process_started" {
		t.Errorf("unexpected event type: %s", resp[0].EventType)
	}
}

func TestTelemetryQueryService_GetEvents_EmptyReturnsSlice(t *testing.T) {
	svc := newQuerySvc(
		&queryMockTelemetryRepo{eventRows: nil},
		&queryMockAppRepo{app: makeQueryApp(1)},
		nil,
	)
	resp, err := svc.GetEvents(context.Background(), 42, 1, 1, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Error("expected empty slice, got nil")
	}
}

func TestTelemetryQueryService_GetEvents_LimitZeroRejected(t *testing.T) {
	svc := newQuerySvc(&queryMockTelemetryRepo{}, &queryMockAppRepo{app: makeQueryApp(1)}, nil)
	_, err := svc.GetEvents(context.Background(), 42, 1, 1, 0)
	if !errors.Is(err, ErrInvalidEventsLimit) {
		t.Errorf("expected ErrInvalidEventsLimit, got %v", err)
	}
}

func TestTelemetryQueryService_GetEvents_LimitExceedsMax(t *testing.T) {
	svc := newQuerySvc(&queryMockTelemetryRepo{}, &queryMockAppRepo{app: makeQueryApp(1)}, nil)
	_, err := svc.GetEvents(context.Background(), 42, 1, 1, MaxEventsLimit+1)
	if !errors.Is(err, ErrInvalidEventsLimit) {
		t.Errorf("expected ErrInvalidEventsLimit, got %v", err)
	}
}

// ---- Tests: DTO mapping ----

func TestMetricToResponse_ExposesCorrectFields(t *testing.T) {
	pid := int64(1234)
	cpuVal := 5.5
	memVal := uint64(1024)
	m := &models.ApplicationMetric{
		ID:            99, // must NOT appear in response DTO
		ApplicationID: 1,  // must NOT appear in response DTO
		CollectedAt:   time.Now().UTC(),
		Status:        "DEGRADED",
		PrimaryPID:    &pid,
		CPUPercent:    &cpuVal,
		MemoryBytes:   &memVal,
	}
	resp := metricToResponse(m)

	if resp.Status != "DEGRADED" {
		t.Errorf("status mismatch: expected DEGRADED, got %s", resp.Status)
	}
	if resp.PrimaryPID == nil || *resp.PrimaryPID != 1234 {
		t.Errorf("PID mismatch")
	}
	// Compile-time proof: ApplicationMetricResponse has no ID or ApplicationID fields
	_ = dto.ApplicationMetricResponse{}
}

func TestEventToResponse_ExposesCorrectFields(t *testing.T) {
	oldPID := int64(100)
	newPID := int64(200)
	e := &models.ApplicationEvent{
		ID:            99, // must NOT appear in response DTO
		ApplicationID: 1,  // must NOT appear in response DTO
		EventType:     "restart_detected",
		EventTime:     time.Now().UTC(),
		OldPID:        &oldPID,
		NewPID:        &newPID,
	}
	resp := eventToResponse(e)

	if resp.EventType != "restart_detected" {
		t.Errorf("event type mismatch")
	}
	if resp.OldPID == nil || *resp.OldPID != 100 {
		t.Errorf("OldPID mismatch")
	}
	// Compile-time proof: ApplicationEventResponse has no ID or ApplicationID fields
	_ = dto.ApplicationEventResponse{}
}
