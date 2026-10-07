package service

import (
	"context"
	"errors"
	"math"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"vpsmonitoring-backend/internal/alert/dto"
	"vpsmonitoring-backend/internal/alert/models"
	"vpsmonitoring-backend/internal/alert/repository"
	appRepository "vpsmonitoring-backend/internal/application/repository"
	serverService "vpsmonitoring-backend/internal/server/service"
)

const DefaultAlertLimit = 50
const MaxAlertLimit = 100

var (
	ErrAlertNotFound         = errors.New("alert not found")
	ErrAlertResourceNotFound = errors.New("resource not found")
	ErrInvalidAlertPage      = errors.New("page must be greater than or equal to 1")
	ErrInvalidAlertLimit     = errors.New("limit must be between 1 and 100")
	ErrInvalidAlertStatus    = errors.New("status must be ACTIVE or RESOLVED")
	ErrInvalidAlertSeverity  = errors.New("severity must be INFO, WARNING or CRITICAL")
	ErrInvalidAlertCondition = errors.New("unsupported condition_type")
)

type AlertQueryService interface {
	List(context.Context, int64, dto.AlertListFilter) (*dto.AlertListResponse, error)
	Get(context.Context, int64, uuid.UUID) (*dto.AlertResponse, error)
	ListByServer(context.Context, int64, int64, dto.AlertListFilter) (*dto.AlertListResponse, error)
	ListByApplication(context.Context, int64, int64, int64, dto.AlertListFilter) (*dto.AlertListResponse, error)
}

type alertQueryService struct {
	repo         repository.AlertRepository
	servers      serverService.ServerService
	applications appRepository.ApplicationRepository
}

func NewAlertQueryService(repo repository.AlertRepository, servers serverService.ServerService, applications appRepository.ApplicationRepository) AlertQueryService {
	return &alertQueryService{repo: repo, servers: servers, applications: applications}
}

func validateAlertFilter(filter *dto.AlertListFilter) error {
	if filter.Page < 1 {
		return ErrInvalidAlertPage
	}
	if filter.Limit < 1 || filter.Limit > MaxAlertLimit {
		return ErrInvalidAlertLimit
	}
	if filter.Page > math.MaxInt/filter.Limit {
		return ErrInvalidAlertPage
	}
	filter.Status = strings.ToUpper(strings.TrimSpace(filter.Status))
	filter.Severity = strings.ToUpper(strings.TrimSpace(filter.Severity))
	filter.ConditionType = strings.ToUpper(strings.TrimSpace(filter.ConditionType))
	if filter.Status != "" && filter.Status != models.AlertStatusActive && filter.Status != models.AlertStatusResolved {
		return ErrInvalidAlertStatus
	}
	if filter.Severity != "" && !validSeverities[filter.Severity] {
		return ErrInvalidAlertSeverity
	}
	if filter.ConditionType != "" && !serverConditions[filter.ConditionType] && !applicationConditions[filter.ConditionType] {
		return ErrInvalidAlertCondition
	}
	if filter.ServerID != nil && *filter.ServerID <= 0 {
		return ErrAlertResourceNotFound
	}
	if filter.ApplicationID != nil && *filter.ApplicationID <= 0 {
		return ErrAlertResourceNotFound
	}
	return nil
}

func alertResponse(alert *models.Alert) dto.AlertResponse {
	return dto.AlertResponse{ID: alert.ID, RuleID: alert.RuleID, ServerID: alert.ServerID, ApplicationID: alert.ApplicationID, RuleName: alert.RuleName, Severity: alert.Severity, ConditionType: alert.ConditionType, Status: alert.Status, Message: alert.Message, FirstTriggeredAt: alert.FirstTriggeredAt, LastTriggeredAt: alert.LastTriggeredAt, ResolvedAt: alert.ResolvedAt, ResolutionReason: alert.ResolutionReason, ObservedValue: alert.LastObservedValue, LastObservedState: alert.LastObservedState, CreatedAt: alert.CreatedAt, UpdatedAt: alert.UpdatedAt}
}

func (s *alertQueryService) list(ctx context.Context, userID int64, filter dto.AlertListFilter) (*dto.AlertListResponse, error) {
	if err := validateAlertFilter(&filter); err != nil {
		return nil, err
	}
	query := repository.AlertQuery{UserID: userID, ServerID: filter.ServerID, ApplicationID: filter.ApplicationID, Status: filter.Status, Severity: filter.Severity, ConditionType: filter.ConditionType, Limit: filter.Limit, Offset: (filter.Page - 1) * filter.Limit}
	alerts, total, err := s.repo.List(ctx, query)
	if err != nil {
		return nil, err
	}
	responses := make([]dto.AlertResponse, 0, len(alerts))
	for i := range alerts {
		responses = append(responses, alertResponse(&alerts[i]))
	}
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(filter.Limit) - 1) / int64(filter.Limit))
	}
	return &dto.AlertListResponse{Alerts: responses, Page: filter.Page, Limit: filter.Limit, Total: total, TotalPages: totalPages}, nil
}
func (s *alertQueryService) List(ctx context.Context, userID int64, filter dto.AlertListFilter) (*dto.AlertListResponse, error) {
	return s.list(ctx, userID, filter)
}
func (s *alertQueryService) Get(ctx context.Context, userID int64, id uuid.UUID) (*dto.AlertResponse, error) {
	alert, err := s.repo.GetByIDAndUserID(ctx, id, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAlertNotFound
	}
	if err != nil {
		return nil, err
	}
	response := alertResponse(alert)
	return &response, nil
}
func (s *alertQueryService) ListByServer(ctx context.Context, userID, serverID int64, filter dto.AlertListFilter) (*dto.AlertListResponse, error) {
	if _, err := s.servers.GetServer(ctx, serverID, userID); err != nil {
		return nil, ErrAlertResourceNotFound
	}
	filter.ServerID = &serverID
	filter.ApplicationID = nil
	return s.list(ctx, userID, filter)
}
func (s *alertQueryService) ListByApplication(ctx context.Context, userID, serverID, applicationID int64, filter dto.AlertListFilter) (*dto.AlertListResponse, error) {
	if _, err := s.servers.GetServer(ctx, serverID, userID); err != nil {
		return nil, ErrAlertResourceNotFound
	}
	app, err := s.applications.GetByID(ctx, applicationID)
	if err != nil || app == nil || app.ServerID != serverID {
		return nil, ErrAlertResourceNotFound
	}
	filter.ServerID = &serverID
	filter.ApplicationID = &applicationID
	return s.list(ctx, userID, filter)
}
