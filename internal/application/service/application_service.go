package service

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"gorm.io/gorm"

	"vpsmonitoring-backend/internal/application/dto"
	"vpsmonitoring-backend/internal/application/models"
	"vpsmonitoring-backend/internal/application/repository"
	serverServicePkg "vpsmonitoring-backend/internal/server/service"
)

var (
	ErrServerOwnership       = errors.New("server not found or access denied")
	ErrApplicationNotFound   = errors.New("application not found")
	ErrNameRequired          = errors.New("name is required and must be between 2 and 255 characters")
	ErrInvalidMatchType      = errors.New("match_type must be 'systemd_unit' or 'exe_path'")
	ErrMatchValueRequired    = errors.New("match_value is required and must not be empty")
	ErrInvalidPort           = errors.New("monitor_port must be between 1 and 65535")
	ErrInvalidHTTPURL        = errors.New("monitor_http_url must be a valid http:// or https:// URL")
	ErrInvalidLogSourceType  = errors.New("log_source_type must be 'nginx_access', 'apache_access' or null")
	ErrLogSourcePathRequired = errors.New("log_source_path is required when log_source_type is provided")
	ErrLogSourcePathInvalid  = errors.New("log_source_path cannot be provided without a valid log_source_type")
	ErrDuplicateIdentity     = errors.New("an application with this identity already exists on this server")
	ErrEnabledLimitReached   = errors.New("maximum of 15 enabled applications per server reached")
)

type ApplicationService interface {
	Create(ctx context.Context, userID, serverID int64, req dto.ApplicationCreateRequest) (*dto.ApplicationResponse, error)
	List(ctx context.Context, userID, serverID int64) ([]dto.ApplicationResponse, error)
	Get(ctx context.Context, userID, serverID, appID int64) (*dto.ApplicationResponse, error)
	Update(ctx context.Context, userID, serverID, appID int64, req dto.ApplicationUpdateRequest) (*dto.ApplicationResponse, error)
	Delete(ctx context.Context, userID, serverID, appID int64) error
	GetAgentConfig(ctx context.Context, serverID int64) ([]dto.ApplicationResponse, error)
}

type applicationService struct {
	repo          repository.ApplicationRepository
	serverService serverServicePkg.ServerService
	db            *gorm.DB
}

func NewApplicationService(repo repository.ApplicationRepository, serverService serverServicePkg.ServerService, db *gorm.DB) ApplicationService {
	return &applicationService{
		repo:          repo,
		serverService: serverService,
		db:            db,
	}
}

func (s *applicationService) validateServerOwnership(ctx context.Context, userID, serverID int64) error {
	_, err := s.serverService.GetServer(ctx, serverID, userID)
	if err != nil {
		if errors.Is(err, serverServicePkg.ErrServerNotFound) {
			return ErrServerOwnership
		}
		return err
	}
	return nil
}

func validateConfig(name, matchType, matchValue string, port *int, httpURL *string, logType *string, logPath *string) error {
	name = strings.TrimSpace(name)
	if len(name) < 2 || len(name) > 255 {
		return ErrNameRequired
	}

	if matchType != models.MatchTypeSystemdUnit && matchType != models.MatchTypeExePath {
		return ErrInvalidMatchType
	}

	matchValue = strings.TrimSpace(matchValue)
	if matchValue == "" {
		return ErrMatchValueRequired
	}

	if port != nil {
		if *port < 1 || *port > 65535 {
			return ErrInvalidPort
		}
	}

	if httpURL != nil {
		parsed, err := url.ParseRequestURI(*httpURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return ErrInvalidHTTPURL
		}
	}

	if logType != nil {
		if *logType != models.LogSourceTypeNginxAccess && *logType != models.LogSourceTypeApacheAccess {
			return ErrInvalidLogSourceType
		}
		if logPath == nil || strings.TrimSpace(*logPath) == "" {
			return ErrLogSourcePathRequired
		}
	} else {
		if logPath != nil && strings.TrimSpace(*logPath) != "" {
			return ErrLogSourcePathInvalid
		}
	}

	return nil
}

func toResponse(app *models.Application) dto.ApplicationResponse {
	return dto.ApplicationResponse{
		ID:             app.ID,
		ServerID:       app.ServerID,
		Name:           app.Name,
		IsEnabled:      app.IsEnabled,
		MatchType:      app.MatchType,
		MatchValue:     app.MatchValue,
		MonitorPort:    app.MonitorPort,
		MonitorHTTPURL: app.MonitorHTTPURL,
		LogSourceType:  app.LogSourceType,
		LogSourcePath:  app.LogSourcePath,
		CreatedAt:      app.CreatedAt,
		UpdatedAt:      app.UpdatedAt,
	}
}

func (s *applicationService) Create(ctx context.Context, userID, serverID int64, req dto.ApplicationCreateRequest) (*dto.ApplicationResponse, error) {
	if err := s.validateServerOwnership(ctx, userID, serverID); err != nil {
		return nil, err
	}

	var monitorPort *int
	if req.MonitorPort > 0 {
		monitorPort = &req.MonitorPort
	}
	var monitorHTTPURL *string
	if req.MonitorHTTPURL != "" {
		monitorHTTPURL = &req.MonitorHTTPURL
	}
	var logSourceType *string
	if req.LogSourceType != "" {
		logSourceType = &req.LogSourceType
	}
	var logSourcePath *string
	if req.LogSourcePath != "" {
		logSourcePath = &req.LogSourcePath
	}

	if err := validateConfig(req.Name, req.MatchType, req.MatchValue, monitorPort, monitorHTTPURL, logSourceType, logSourcePath); err != nil {
		return nil, err
	}

	// Transaction to check limits and duplicates safely
	var newApp *models.Application
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepo := repository.NewApplicationRepository(tx)

		// Check duplicate identity
		existing, err := txRepo.GetByServerAndIdentity(ctx, serverID, req.MatchType, req.MatchValue)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if existing != nil {
			return ErrDuplicateIdentity
		}

		// Check limit (creating apps are enabled by default)
		count, err := txRepo.CountEnabledByServerID(ctx, serverID)
		if err != nil {
			return err
		}
		if count >= 15 {
			return ErrEnabledLimitReached
		}

		app := &models.Application{
			ServerID:       serverID,
			Name:           strings.TrimSpace(req.Name),
			IsEnabled:      true,
			MatchType:      req.MatchType,
			MatchValue:     strings.TrimSpace(req.MatchValue),
			MonitorPort:    monitorPort,
			MonitorHTTPURL: monitorHTTPURL,
			LogSourceType:  logSourceType,
			LogSourcePath:  logSourcePath,
		}

		if err := txRepo.Create(ctx, app); err != nil {
			return err
		}
		newApp = app
		return nil
	})

	if err != nil {
		return nil, err
	}

	resp := toResponse(newApp)
	return &resp, nil
}

func (s *applicationService) List(ctx context.Context, userID, serverID int64) ([]dto.ApplicationResponse, error) {
	if err := s.validateServerOwnership(ctx, userID, serverID); err != nil {
		return nil, err
	}

	apps, err := s.repo.ListByServerID(ctx, serverID)
	if err != nil {
		return nil, err
	}

	var responses []dto.ApplicationResponse
	for _, app := range apps {
		responses = append(responses, toResponse(&app))
	}
	return responses, nil
}

func (s *applicationService) Get(ctx context.Context, userID, serverID, appID int64) (*dto.ApplicationResponse, error) {
	if err := s.validateServerOwnership(ctx, userID, serverID); err != nil {
		return nil, err
	}

	app, err := s.repo.GetByID(ctx, appID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrApplicationNotFound
		}
		return nil, err
	}

	if app.ServerID != serverID {
		return nil, ErrApplicationNotFound
	}

	resp := toResponse(app)
	return &resp, nil
}

func (s *applicationService) Update(ctx context.Context, userID, serverID, appID int64, req dto.ApplicationUpdateRequest) (*dto.ApplicationResponse, error) {
	if err := s.validateServerOwnership(ctx, userID, serverID); err != nil {
		return nil, err
	}

	var updatedApp *models.Application
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepo := repository.NewApplicationRepository(tx)

		app, err := txRepo.GetByID(ctx, appID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrApplicationNotFound
			}
			return err
		}

		if app.ServerID != serverID {
			return ErrApplicationNotFound
		}

		// Apply updates
		if req.Name != nil {
			app.Name = strings.TrimSpace(*req.Name)
		}

		newMatchType := app.MatchType
		if req.MatchType != nil {
			newMatchType = *req.MatchType
		}

		newMatchValue := app.MatchValue
		if req.MatchValue != nil {
			newMatchValue = strings.TrimSpace(*req.MatchValue)
		}

		// Re-validate complete config
		port := app.MonitorPort
		if req.MonitorPort != nil {
			port = req.MonitorPort
		}
		httpURL := app.MonitorHTTPURL
		if req.MonitorHTTPURL != nil {
			httpURL = req.MonitorHTTPURL
		}
		logType := app.LogSourceType
		if req.LogSourceType != nil {
			logType = req.LogSourceType
		}
		logPath := app.LogSourcePath
		if req.LogSourcePath != nil {
			logPath = req.LogSourcePath
		}

		if err := validateConfig(app.Name, newMatchType, newMatchValue, port, httpURL, logType, logPath); err != nil {
			return err
		}

		// Check duplicate if identity changed
		if newMatchType != app.MatchType || newMatchValue != app.MatchValue {
			existing, err := txRepo.GetByServerAndIdentity(ctx, serverID, newMatchType, newMatchValue)
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if existing != nil && existing.ID != appID {
				return ErrDuplicateIdentity
			}
		}

		app.MatchType = newMatchType
		app.MatchValue = newMatchValue
		app.MonitorPort = port
		app.MonitorHTTPURL = httpURL
		app.LogSourceType = logType
		app.LogSourcePath = logPath

		if req.IsEnabled != nil && *req.IsEnabled != app.IsEnabled {
			if *req.IsEnabled {
				count, err := txRepo.CountEnabledByServerID(ctx, serverID)
				if err != nil {
					return err
				}
				if count >= 15 {
					return ErrEnabledLimitReached
				}
			}
			app.IsEnabled = *req.IsEnabled
		}

		if err := txRepo.Update(ctx, app); err != nil {
			return err
		}
		updatedApp = app
		return nil
	})

	if err != nil {
		return nil, err
	}

	resp := toResponse(updatedApp)
	return &resp, nil
}

func (s *applicationService) Delete(ctx context.Context, userID, serverID, appID int64) error {
	if err := s.validateServerOwnership(ctx, userID, serverID); err != nil {
		return err
	}

	app, err := s.repo.GetByID(ctx, appID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrApplicationNotFound
		}
		return err
	}

	if app.ServerID != serverID {
		return ErrApplicationNotFound
	}

	return s.repo.Delete(ctx, appID)
}

func (s *applicationService) GetAgentConfig(ctx context.Context, serverID int64) ([]dto.ApplicationResponse, error) {
	// The agent controller is responsible for authentication and ensuring
	// the agent has permission to access this serverID. We just retrieve the enabled apps.
	apps, err := s.repo.ListEnabledByServerID(ctx, serverID)
	if err != nil {
		return nil, err
	}

	var res []dto.ApplicationResponse
	for _, a := range apps {
		res = append(res, toResponse(&a))
	}

	if res == nil {
		res = []dto.ApplicationResponse{}
	}

	return res, nil
}
