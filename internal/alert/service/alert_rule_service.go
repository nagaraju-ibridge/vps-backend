package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"vpsmonitoring-backend/internal/alert/dto"
	"vpsmonitoring-backend/internal/alert/models"
	"vpsmonitoring-backend/internal/alert/repository"
	appRepository "vpsmonitoring-backend/internal/application/repository"
	serverService "vpsmonitoring-backend/internal/server/service"
)

const MaxDurationSeconds = 86400 * 30

var (
	ErrRuleNotFound       = errors.New("alert rule not found")
	ErrInvalidName        = errors.New("name must be between 2 and 255 characters")
	ErrInvalidScope       = errors.New("scope_type must be SERVER or APPLICATION")
	ErrInvalidCondition   = errors.New("unsupported condition_type")
	ErrInvalidOperator    = errors.New("unsupported operator")
	ErrInvalidSeverity    = errors.New("severity must be INFO, WARNING or CRITICAL")
	ErrInvalidDuration    = errors.New("duration_seconds must be between 0 and 2592000")
	ErrInvalidThreshold   = errors.New("invalid threshold for condition_type")
	ErrInvalidCombination = errors.New("condition_type is not valid for the selected scope")
	ErrApplicationNeeded  = errors.New("application_id is required for APPLICATION scope")
	ErrApplicationTarget  = errors.New("application not found or access denied")
	ErrServerTarget       = errors.New("server not found or access denied")
	ErrStatusRequired     = errors.New("is_enabled is required")
)

type AlertRuleService interface {
	Create(context.Context, int64, dto.CreateAlertRuleRequest) (*dto.AlertRuleResponse, error)
	List(context.Context, int64) ([]dto.AlertRuleResponse, error)
	Get(context.Context, int64, int64) (*dto.AlertRuleResponse, error)
	Update(context.Context, int64, int64, dto.UpdateAlertRuleRequest) (*dto.AlertRuleResponse, error)
	UpdateStatus(context.Context, int64, int64, dto.UpdateAlertRuleStatusRequest) (*dto.AlertRuleResponse, error)
	Delete(context.Context, int64, int64) error
}

type alertRuleService struct {
	repo         repository.AlertRuleRepository
	servers      serverService.ServerService
	applications appRepository.ApplicationRepository
}

func NewAlertRuleService(repo repository.AlertRuleRepository, servers serverService.ServerService, applications appRepository.ApplicationRepository) AlertRuleService {
	return &alertRuleService{repo: repo, servers: servers, applications: applications}
}

var serverConditions = map[string]bool{models.ConditionServerCPU: true, models.ConditionServerMemory: true, models.ConditionServerDisk: true, models.ConditionServerOffline: true}
var applicationConditions = map[string]bool{models.ConditionApplicationDown: true, models.ConditionApplicationDegrade: true, models.ConditionHTTPHealthFailure: true, models.ConditionHTTPResponseTime: true, models.ConditionHTTP4XX: true, models.ConditionHTTP5XX: true, models.ConditionApplicationRestart: true}
var numericConditions = map[string]bool{models.ConditionServerCPU: true, models.ConditionServerMemory: true, models.ConditionServerDisk: true, models.ConditionHTTPResponseTime: true, models.ConditionHTTP4XX: true, models.ConditionHTTP5XX: true}
var stateThresholds = map[string]string{models.ConditionServerOffline: "OFFLINE", models.ConditionApplicationDown: "DOWN", models.ConditionApplicationDegrade: "DEGRADED", models.ConditionHTTPHealthFailure: "DOWN", models.ConditionApplicationRestart: "TRUE"}
var validOperators = map[string]bool{models.OperatorGT: true, models.OperatorGTE: true, models.OperatorLT: true, models.OperatorLTE: true, models.OperatorEQ: true, models.OperatorNEQ: true}
var validSeverities = map[string]bool{models.SeverityInfo: true, models.SeverityWarning: true, models.SeverityCritical: true}

func normalize(value string) string { return strings.ToUpper(strings.TrimSpace(value)) }

func thresholdString(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed), nil
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), nil
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32), nil
	case int:
		return strconv.Itoa(typed), nil
	case int64:
		return strconv.FormatInt(typed, 10), nil
	case json.Number:
		if _, err := typed.Float64(); err != nil {
			return "", ErrInvalidThreshold
		}
		return typed.String(), nil
	default:
		return "", ErrInvalidThreshold
	}
}

func validateRule(rule *models.AlertRule) error {
	rule.Name = strings.TrimSpace(rule.Name)
	rule.ScopeType, rule.ConditionType, rule.Operator, rule.Severity = normalize(rule.ScopeType), normalize(rule.ConditionType), normalize(rule.Operator), normalize(rule.Severity)
	rule.Threshold = strings.TrimSpace(rule.Threshold)
	if len(rule.Name) < 2 || len(rule.Name) > 255 {
		return ErrInvalidName
	}
	if rule.ServerID <= 0 {
		return ErrServerTarget
	}
	if rule.ScopeType != models.ScopeServer && rule.ScopeType != models.ScopeApplication {
		return ErrInvalidScope
	}
	if !serverConditions[rule.ConditionType] && !applicationConditions[rule.ConditionType] {
		return ErrInvalidCondition
	}
	if !validOperators[rule.Operator] {
		return ErrInvalidOperator
	}
	if !validSeverities[rule.Severity] {
		return ErrInvalidSeverity
	}
	if rule.DurationSeconds < 0 || rule.DurationSeconds > MaxDurationSeconds {
		return ErrInvalidDuration
	}
	if rule.ScopeType == models.ScopeServer {
		if !serverConditions[rule.ConditionType] {
			return ErrInvalidCombination
		}
		rule.ApplicationID = nil
	} else {
		if !applicationConditions[rule.ConditionType] {
			return ErrInvalidCombination
		}
		if rule.ApplicationID == nil || *rule.ApplicationID <= 0 {
			return ErrApplicationNeeded
		}
	}
	if numericConditions[rule.ConditionType] {
		value, err := strconv.ParseFloat(rule.Threshold, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return ErrInvalidThreshold
		}
		if (rule.ConditionType == models.ConditionServerCPU || rule.ConditionType == models.ConditionServerMemory || rule.ConditionType == models.ConditionServerDisk) && value > 100 {
			return ErrInvalidThreshold
		}
	} else {
		if rule.Operator != models.OperatorEQ && rule.Operator != models.OperatorNEQ {
			return ErrInvalidOperator
		}
		expected := stateThresholds[rule.ConditionType]
		if normalize(rule.Threshold) != expected {
			return ErrInvalidThreshold
		}
		rule.Threshold = expected
	}
	return nil
}

func (s *alertRuleService) validateOwnership(ctx context.Context, userID int64, rule *models.AlertRule) error {
	if _, err := s.servers.GetServer(ctx, rule.ServerID, userID); err != nil {
		return ErrServerTarget
	}
	if rule.ScopeType == models.ScopeApplication {
		app, err := s.applications.GetByID(ctx, *rule.ApplicationID)
		if err != nil || app == nil || app.ServerID != rule.ServerID {
			return ErrApplicationTarget
		}
	}
	return nil
}

func response(rule *models.AlertRule) *dto.AlertRuleResponse {
	return &dto.AlertRuleResponse{ID: rule.ID, Name: rule.Name, ScopeType: rule.ScopeType, ServerID: rule.ServerID, ApplicationID: rule.ApplicationID, ConditionType: rule.ConditionType, Operator: rule.Operator, Threshold: rule.Threshold, DurationSeconds: rule.DurationSeconds, Severity: rule.Severity, IsEnabled: rule.IsEnabled, CreatedAt: rule.CreatedAt, UpdatedAt: rule.UpdatedAt}
}

func (s *alertRuleService) Create(ctx context.Context, userID int64, req dto.CreateAlertRuleRequest) (*dto.AlertRuleResponse, error) {
	enabled := true
	if req.IsEnabled != nil {
		enabled = *req.IsEnabled
	}
	threshold, err := thresholdString(req.Threshold)
	if err != nil {
		return nil, err
	}
	rule := &models.AlertRule{UserID: userID, ServerID: req.ServerID, ApplicationID: req.ApplicationID, Name: req.Name, ScopeType: req.ScopeType, ConditionType: req.ConditionType, Operator: req.Operator, Threshold: threshold, DurationSeconds: req.DurationSeconds, Severity: req.Severity, IsEnabled: enabled}
	if err := validateRule(rule); err != nil {
		return nil, err
	}
	if err := s.validateOwnership(ctx, userID, rule); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, rule); err != nil {
		return nil, err
	}
	return response(rule), nil
}

func (s *alertRuleService) List(ctx context.Context, userID int64) ([]dto.AlertRuleResponse, error) {
	rules, err := s.repo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]dto.AlertRuleResponse, 0, len(rules))
	for i := range rules {
		result = append(result, *response(&rules[i]))
	}
	return result, nil
}

func (s *alertRuleService) find(ctx context.Context, userID, id int64) (*models.AlertRule, error) {
	rule, err := s.repo.GetByIDAndUserID(ctx, id, userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrRuleNotFound
	}
	return rule, err
}
func (s *alertRuleService) Get(ctx context.Context, userID, id int64) (*dto.AlertRuleResponse, error) {
	rule, err := s.find(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	return response(rule), nil
}
func (s *alertRuleService) Update(ctx context.Context, userID, id int64, req dto.UpdateAlertRuleRequest) (*dto.AlertRuleResponse, error) {
	rule, err := s.find(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if req.Name != nil {
		rule.Name = *req.Name
	}
	if req.ScopeType != nil {
		rule.ScopeType = *req.ScopeType
	}
	if req.ServerID != nil {
		rule.ServerID = *req.ServerID
	}
	if req.ApplicationID != nil {
		rule.ApplicationID = req.ApplicationID
	}
	if req.ConditionType != nil {
		rule.ConditionType = *req.ConditionType
	}
	if req.Operator != nil {
		rule.Operator = *req.Operator
	}
	if req.Threshold != nil {
		threshold, thresholdErr := thresholdString(req.Threshold)
		if thresholdErr != nil {
			return nil, thresholdErr
		}
		rule.Threshold = threshold
	}
	if req.DurationSeconds != nil {
		rule.DurationSeconds = *req.DurationSeconds
	}
	if req.Severity != nil {
		rule.Severity = *req.Severity
	}
	if req.IsEnabled != nil {
		rule.IsEnabled = *req.IsEnabled
	}
	if err = validateRule(rule); err != nil {
		return nil, err
	}
	if err = s.validateOwnership(ctx, userID, rule); err != nil {
		return nil, err
	}
	if err = s.repo.Update(ctx, rule); err != nil {
		return nil, err
	}
	return response(rule), nil
}
func (s *alertRuleService) UpdateStatus(ctx context.Context, userID, id int64, req dto.UpdateAlertRuleStatusRequest) (*dto.AlertRuleResponse, error) {
	if req.IsEnabled == nil {
		return nil, ErrStatusRequired
	}
	rule, err := s.find(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	rule.IsEnabled = *req.IsEnabled
	if err = s.repo.Update(ctx, rule); err != nil {
		return nil, err
	}
	return response(rule), nil
}
func (s *alertRuleService) Delete(ctx context.Context, userID, id int64) error {
	rule, err := s.find(ctx, userID, id)
	if err != nil {
		return err
	}
	return s.repo.Delete(ctx, rule)
}
