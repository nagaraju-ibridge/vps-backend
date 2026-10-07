package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"vpsmonitoring-backend/internal/application/dto"
	"vpsmonitoring-backend/internal/application/models"
	"vpsmonitoring-backend/internal/application/repository"
	serverServicePkg "vpsmonitoring-backend/internal/server/service"
)

const (
	// MaxTelemetryHistoryRange is the maximum allowed time range for history queries.
	MaxTelemetryHistoryRange = 31 * 24 * time.Hour
	// MaxTelemetryHistoryLimit is the maximum number of records for history queries.
	MaxTelemetryHistoryLimit = 44640
	// DefaultTelemetryHistoryLimit is the default if limit is not provided.
	DefaultTelemetryHistoryLimit = 10080

	// MaxEventsLimit is the maximum number of events returnable per query.
	MaxEventsLimit = 100
	// DefaultEventsLimit is the default if limit is not provided.
	DefaultEventsLimit = 50
)

var (
	ErrTelemetryNotFound   = errors.New("no telemetry found for this application")
	ErrInvalidTimeRange    = errors.New("start timestamp must be before or equal to end timestamp")
	ErrTimeRangeTooLarge   = errors.New(fmt.Sprintf("historical range exceeds maximum allowed duration of %s", MaxTelemetryHistoryRange))
	ErrInvalidHistoryLimit = fmt.Errorf("limit must be between 1 and %d", MaxTelemetryHistoryLimit)
	ErrInvalidEventsLimit  = fmt.Errorf("limit must be between 1 and %d", MaxEventsLimit)
)

// TelemetryQueryService handles business logic for the user-facing telemetry read API.
type TelemetryQueryService interface {
	GetLatest(ctx context.Context, userID, serverID, appID int64) (*dto.ApplicationMetricResponse, error)
	GetHistory(ctx context.Context, userID, serverID, appID int64, start, end time.Time, limit int) ([]dto.ApplicationMetricResponse, error)
	GetEvents(ctx context.Context, userID, serverID, appID int64, limit int) ([]dto.ApplicationEventResponse, error)
}

type telemetryQueryService struct {
	queryRepo     repository.TelemetryQueryRepository
	appRepo       repository.ApplicationRepository
	serverService serverServicePkg.ServerService
}

func NewTelemetryQueryService(
	queryRepo repository.TelemetryQueryRepository,
	appRepo repository.ApplicationRepository,
	serverService serverServicePkg.ServerService,
) TelemetryQueryService {
	return &telemetryQueryService{
		queryRepo:     queryRepo,
		appRepo:       appRepo,
		serverService: serverService,
	}
}

// validateOwnership checks that the authenticated user owns the server
// and that the application belongs to that server.
// Always returns ErrServerOwnership or ErrApplicationNotFound on mismatch
// to prevent resource enumeration.
func (s *telemetryQueryService) validateOwnership(ctx context.Context, userID, serverID, appID int64) error {
	// 1. Verify user owns server
	_, err := s.serverService.GetServer(ctx, serverID, userID)
	if err != nil {
		return ErrServerOwnership
	}

	// 2. Verify app exists and belongs to server
	app, err := s.appRepo.GetByID(ctx, appID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrApplicationNotFound
		}
		return err
	}
	if app.ServerID != serverID {
		return ErrApplicationNotFound
	}

	return nil
}

func (s *telemetryQueryService) GetLatest(ctx context.Context, userID, serverID, appID int64) (*dto.ApplicationMetricResponse, error) {
	if err := s.validateOwnership(ctx, userID, serverID, appID); err != nil {
		return nil, err
	}

	m, err := s.queryRepo.GetLatestMetric(ctx, appID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTelemetryNotFound
		}
		return nil, err
	}

	resp := metricToResponse(m)
	return &resp, nil
}

func (s *telemetryQueryService) GetHistory(ctx context.Context, userID, serverID, appID int64, start, end time.Time, limit int) ([]dto.ApplicationMetricResponse, error) {
	if start.After(end) {
		return nil, ErrInvalidTimeRange
	}
	if end.Sub(start) > MaxTelemetryHistoryRange {
		return nil, ErrTimeRangeTooLarge
	}
	if limit <= 0 || limit > MaxTelemetryHistoryLimit {
		return nil, ErrInvalidHistoryLimit
	}

	if err := s.validateOwnership(ctx, userID, serverID, appID); err != nil {
		return nil, err
	}

	metrics, err := s.queryRepo.GetMetricHistory(ctx, appID, start, end, limit)
	if err != nil {
		return nil, err
	}

	// Always return an empty slice, never nil
	result := make([]dto.ApplicationMetricResponse, 0, len(metrics))
	for i := range metrics {
		result = append(result, metricToResponse(&metrics[i]))
	}
	return result, nil
}

func (s *telemetryQueryService) GetEvents(ctx context.Context, userID, serverID, appID int64, limit int) ([]dto.ApplicationEventResponse, error) {
	if limit <= 0 || limit > MaxEventsLimit {
		return nil, ErrInvalidEventsLimit
	}

	if err := s.validateOwnership(ctx, userID, serverID, appID); err != nil {
		return nil, err
	}

	events, err := s.queryRepo.GetEvents(ctx, appID, limit)
	if err != nil {
		return nil, err
	}

	// Always return an empty slice, never nil
	result := make([]dto.ApplicationEventResponse, 0, len(events))
	for i := range events {
		result = append(result, eventToResponse(&events[i]))
	}
	return result, nil
}

// metricToResponse maps an ApplicationMetric model to the safe user-facing DTO.
// Deliberately does NOT expose ID, ApplicationID, or any internal relation keys.
func metricToResponse(m *models.ApplicationMetric) dto.ApplicationMetricResponse {
	return dto.ApplicationMetricResponse{
		CollectedAt:          m.CollectedAt,
		Status:               m.Status,
		ProcessMatched:       m.ProcessMatched,
		PrimaryPID:           m.PrimaryPID,
		ProcessCount:         m.ProcessCount,
		PrimaryStartTime:     m.PrimaryStartTime,
		CPUPercent:           m.CPUPercent,
		MemoryBytes:          m.MemoryBytes,
		PortConfigured:       m.PortConfigured,
		PortListening:        m.PortListening,
		HTTPConfigured:       m.HTTPConfigured,
		HTTPAvailable:        m.HTTPAvailable,
		HTTPStatusCode:       m.HTTPStatusCode,
		HTTPLatencyMs:        m.HTTPLatencyMs,
		HTTPErrorClass:       m.HTTPErrorClass,
		LogConfigured:        m.LogConfigured,
		LogAvailable:         m.LogAvailable,
		TotalRequests:        m.TotalRequests,
		RequestsPerSecond:    m.RequestsPerSecond,
		Status2xx:            m.Status2xx,
		Status3xx:            m.Status3xx,
		Status4xx:            m.Status4xx,
		Status5xx:            m.Status5xx,
		StatusOther:          m.StatusOther,
		ActiveResponseCount:  m.ActiveResponseCount,
		ActiveResponseAvgMs:  m.ActiveResponseAvgMs,
		PassiveResponseCount: m.PassiveResponseCount,
		PassiveResponseAvgMs: m.PassiveResponseAvgMs,
	}
}

// eventToResponse maps an ApplicationEvent model to the safe user-facing DTO.
// Deliberately does NOT expose ID or ApplicationID.
func eventToResponse(e *models.ApplicationEvent) dto.ApplicationEventResponse {
	return dto.ApplicationEventResponse{
		EventTime: e.EventTime,
		EventType: e.EventType,
		OldPID:    e.OldPID,
		NewPID:    e.NewPID,
		Details:   e.Details,
	}
}
