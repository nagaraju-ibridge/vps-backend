package service

import (
	"context"
	"errors"
	"strings"

	"vpsmonitoring-backend/internal/server/dto"
	"vpsmonitoring-backend/internal/server/models"
	"vpsmonitoring-backend/internal/server/repository"
)

var (
	ErrServerNotFound    = errors.New("server not found")
	ErrInvalidServerName = errors.New("server name is required")
	ErrNameLength        = errors.New("server name must be between 2 and 100 characters")
)

// ServerService defines business logic operations for server management
type ServerService interface {
	CreateServer(ctx context.Context, userID int64, req dto.CreateServerRequest) (*dto.ServerResponse, error)
	GetUserServers(ctx context.Context, userID int64) ([]dto.ServerResponse, error)
	GetServer(ctx context.Context, id int64, userID int64) (*dto.ServerResponse, error)
	UpdateServer(ctx context.Context, id int64, userID int64, req dto.UpdateServerRequest) (*dto.ServerResponse, error)
	DeleteServer(ctx context.Context, id int64, userID int64) error
}

type serverService struct {
	repo repository.ServerRepository
}

// NewServerService initializes a new ServerService instance
func NewServerService(repo repository.ServerRepository) ServerService {
	return &serverService{repo: repo}
}

func (s *serverService) CreateServer(ctx context.Context, userID int64, req dto.CreateServerRequest) (*dto.ServerResponse, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, ErrInvalidServerName
	}
	if len(name) < 2 || len(name) > 100 {
		return nil, ErrNameLength
	}

	newServer := models.Server{
		UserID:      userID,
		Name:        name,
		AgentStatus: models.StatusPending,
	}

	if err := s.repo.Create(ctx, &newServer); err != nil {
		return nil, err
	}

	res := dto.ToServerResponse(&newServer)
	return &res, nil
}

func (s *serverService) GetUserServers(ctx context.Context, userID int64) ([]dto.ServerResponse, error) {
	servers, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	responses := make([]dto.ServerResponse, len(servers))
	for i, srv := range servers {
		responses[i] = dto.ToServerResponse(&srv)
	}
	return responses, nil
}

func (s *serverService) GetServer(ctx context.Context, id int64, userID int64) (*dto.ServerResponse, error) {
	server, err := s.repo.GetByIDAndUserID(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if server == nil {
		return nil, ErrServerNotFound
	}

	res := dto.ToServerResponse(server)
	return &res, nil
}

func (s *serverService) UpdateServer(ctx context.Context, id int64, userID int64, req dto.UpdateServerRequest) (*dto.ServerResponse, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, ErrInvalidServerName
	}
	if len(name) < 2 || len(name) > 100 {
		return nil, ErrNameLength
	}

	server, err := s.repo.GetByIDAndUserID(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if server == nil {
		return nil, ErrServerNotFound
	}

	// Strictly allow updating the server name only
	server.Name = name

	if err := s.repo.Update(ctx, server); err != nil {
		return nil, err
	}

	res := dto.ToServerResponse(server)
	return &res, nil
}

func (s *serverService) DeleteServer(ctx context.Context, id int64, userID int64) error {
	// Check server existence & user ownership
	server, err := s.repo.GetByIDAndUserID(ctx, id, userID)
	if err != nil {
		return err
	}
	if server == nil {
		return ErrServerNotFound
	}

	// Soft delete through repository
	return s.repo.Delete(ctx, id, userID)
}
