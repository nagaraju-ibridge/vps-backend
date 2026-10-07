package service

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"vpsmonitoring-backend/internal/health/dto"
	"vpsmonitoring-backend/internal/health/models"
	"vpsmonitoring-backend/internal/health/repository"
	serverService "vpsmonitoring-backend/internal/server/service"
)

var (
	ErrServerNotFound    = errors.New("server not found or access denied")
	ErrInvalidURL        = errors.New("invalid or malformed URL")
	ErrUnsupportedScheme = errors.New("unsupported URL scheme; only http and https are allowed")
	ErrMissingHostname   = errors.New("URL must contain a valid hostname")
	ErrInvalidInterval   = errors.New("interval must be between 10 and 3600 seconds")
	ErrTooManyChecks     = errors.New("maximum number of health checks (10) reached for this server")
	ErrConfigNotFound    = repository.ErrConfigNotFound
	ErrInvalidPayload    = errors.New("invalid payload or timestamp")
)

const (
	MaxChecksPerServer = 10
	MinIntervalSec     = 10
	MaxIntervalSec     = 3600
	DefaultIntervalSec = 60
)

type HealthService interface {
	CreateConfig(ctx context.Context, serverID, userID int64, req dto.CreateHealthConfigRequest) (*dto.HttpHealthConfigDTO, error)
	ListConfigs(ctx context.Context, serverID, userID int64) ([]dto.HttpHealthConfigDTO, error)
	DeleteConfig(ctx context.Context, serverID, userID, configID int64) error
	GetAgentConfig(ctx context.Context, serverID int64) ([]dto.HttpHealthConfigDTO, error)
	IngestHealthChecks(ctx context.Context, serverID int64, payload dto.AgentHealthCheckPayload) error
	GetHealthChecks(ctx context.Context, serverID, userID int64) ([]dto.HealthCheckResultDTO, error)
}

type healthService struct {
	repo      repository.HealthRepository
	serverSvc serverService.ServerService
	cache     *HealthResultCache
}

func NewHealthService(repo repository.HealthRepository, serverSvc serverService.ServerService, cache *HealthResultCache) HealthService {
	return &healthService{repo: repo, serverSvc: serverSvc, cache: cache}
}

func (s *healthService) CreateConfig(ctx context.Context, serverID, userID int64, req dto.CreateHealthConfigRequest) (*dto.HttpHealthConfigDTO, error) {
	// Verify ownership
	server, err := s.serverSvc.GetServer(ctx, serverID, userID)
	if err != nil || server == nil {
		return nil, ErrServerNotFound
	}

	// Validate URL
	parsedURL, err := url.ParseRequestURI(req.URL)
	if err != nil {
		return nil, ErrInvalidURL
	}

	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, ErrUnsupportedScheme
	}

	hostname := strings.TrimSpace(parsedURL.Hostname())
	if hostname == "" {
		return nil, ErrMissingHostname
	}

	// Validate Interval
	interval := DefaultIntervalSec
	if req.IntervalSec != nil {
		interval = *req.IntervalSec
	}
	if interval < MinIntervalSec || interval > MaxIntervalSec {
		return nil, ErrInvalidInterval
	}

	// Validate Limit
	count, err := s.repo.CountByServerID(serverID)
	if err != nil {
		return nil, err
	}
	if count >= MaxChecksPerServer {
		return nil, ErrTooManyChecks
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	config := &models.HttpHealthConfig{
		ServerID:    serverID,
		URL:         req.URL, // Raw URL
		IntervalSec: interval,
		IsActive:    isActive,
	}

	if err := s.repo.Create(config); err != nil {
		return nil, err
	}

	return mapModelToDTO(config), nil
}

func (s *healthService) ListConfigs(ctx context.Context, serverID, userID int64) ([]dto.HttpHealthConfigDTO, error) {
	server, err := s.serverSvc.GetServer(ctx, serverID, userID)
	if err != nil || server == nil {
		return nil, ErrServerNotFound
	}

	configs, err := s.repo.ListByServerID(serverID)
	if err != nil {
		return nil, err
	}

	var dtos []dto.HttpHealthConfigDTO
	for _, c := range configs {
		dtos = append(dtos, *mapModelToDTO(&c))
	}

	if dtos == nil {
		dtos = []dto.HttpHealthConfigDTO{}
	}

	return dtos, nil
}

func (s *healthService) DeleteConfig(ctx context.Context, serverID, userID, configID int64) error {
	server, err := s.serverSvc.GetServer(ctx, serverID, userID)
	if err != nil || server == nil {
		return ErrServerNotFound
	}

	// Verify the config exists and belongs to the server
	config, err := s.repo.GetByID(configID)
	if err != nil {
		return err
	}

	if config.ServerID != serverID {
		return ErrConfigNotFound // Do not expose existence of other server's configs
	}

	return s.repo.Delete(configID)
}

func (s *healthService) GetAgentConfig(ctx context.Context, serverID int64) ([]dto.HttpHealthConfigDTO, error) {
	configs, err := s.repo.ListActiveByServerID(serverID)
	if err != nil {
		return nil, err
	}

	var dtos []dto.HttpHealthConfigDTO
	for _, c := range configs {
		dtos = append(dtos, *mapModelToDTO(&c))
	}

	if dtos == nil {
		dtos = []dto.HttpHealthConfigDTO{}
	}

	return dtos, nil
}

func (s *healthService) IngestHealthChecks(ctx context.Context, serverID int64, payload dto.AgentHealthCheckPayload) error {
	if payload.CollectedAt.IsZero() {
		return ErrInvalidPayload
	}

	// Fetch active configs for this server to validate against
	activeConfigs, err := s.repo.ListActiveByServerID(serverID)
	if err != nil {
		return err
	}

	configMap := make(map[int64]models.HttpHealthConfig)
	for _, c := range activeConfigs {
		configMap[c.ID] = c
	}

	// Validate before inserting to avoid partial cache corruption
	for _, check := range payload.Checks {
		config, exists := configMap[check.ConfigID]
		if !exists {
			return errors.New("invalid config_id for server")
		}
		if check.URL != config.URL {
			return errors.New("url mismatch for config_id")
		}
		if check.LatencyMs < 0 {
			return errors.New("latency cannot be negative")
		}
		if check.StatusCode < 0 {
			return errors.New("status code cannot be negative")
		}
	}

	// Set in cache
	for _, check := range payload.Checks {
		s.cache.Set(serverID, dto.HealthCheckResultDTO{
			ConfigID:    check.ConfigID,
			URL:         check.URL,
			StatusCode:  check.StatusCode,
			LatencyMs:   check.LatencyMs,
			IsAvailable: check.IsAvailable,
			ErrorClass:  check.ErrorClass,
			CollectedAt: payload.CollectedAt,
		})
	}

	return nil
}

func (s *healthService) GetHealthChecks(ctx context.Context, serverID, userID int64) ([]dto.HealthCheckResultDTO, error) {
	server, err := s.serverSvc.GetServer(ctx, serverID, userID)
	if err != nil || server == nil {
		return nil, ErrServerNotFound
	}

	return s.cache.GetByServerID(serverID), nil
}

func mapModelToDTO(config *models.HttpHealthConfig) *dto.HttpHealthConfigDTO {
	return &dto.HttpHealthConfigDTO{
		ID:          config.ID,
		ServerID:    config.ServerID,
		URL:         config.URL,
		IntervalSec: config.IntervalSec,
		IsActive:    config.IsActive,
		CreatedAt:   config.CreatedAt,
		UpdatedAt:   config.UpdatedAt,
	}
}
