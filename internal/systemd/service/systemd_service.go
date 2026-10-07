package service

import (
	"context"
	"errors"
	"sort"
	"strings"

	"vpsmonitoring-backend/internal/server/service"
	"vpsmonitoring-backend/internal/systemd/cache"
	"vpsmonitoring-backend/internal/systemd/dto"
)

var (
	ErrServerNotFound = errors.New("server not found or unmonitored")
	ErrNoSnapshot     = errors.New("no systemd snapshot available for this server")
	ErrInvalidLimit   = errors.New("invalid limit")
	ErrInvalidSort    = errors.New("invalid sort order")
)

type SystemdService interface {
	IngestSnapshot(serverID int64, payload dto.ServicePayloadDTO) error
	GetLatestSnapshot(ctx context.Context, serverID, userID int64, limit int, sortOrder string) (*dto.ServiceResponseDTO, error)
}

type systemdService struct {
	cache     cache.SystemdCache
	serverSvc service.ServerService
}

func NewSystemdService(c cache.SystemdCache, serverSvc service.ServerService) SystemdService {
	return &systemdService{cache: c, serverSvc: serverSvc}
}

func (s *systemdService) IngestSnapshot(serverID int64, payload dto.ServicePayloadDTO) error {
	s.cache.Set(serverID, payload.CollectedAt, payload.Services)
	return nil
}

func (s *systemdService) GetLatestSnapshot(ctx context.Context, serverID, userID int64, limit int, sortOrder string) (*dto.ServiceResponseDTO, error) {
	// Verify server ownership
	server, err := s.serverSvc.GetServer(ctx, serverID, userID)
	if err != nil || server == nil {
		return nil, ErrServerNotFound
	}

	if limit <= 0 || limit > 100 {
		return nil, ErrInvalidLimit
	}

	entry, ok := s.cache.Get(serverID)
	if !ok {
		return nil, ErrNoSnapshot
	}

	services := make([]dto.ServiceSnapshotDTO, len(entry.Services))
	copy(services, entry.Services)

	switch sortOrder {
	case "name_asc":
		sort.Slice(services, func(i, j int) bool {
			return strings.ToLower(services[i].Name) < strings.ToLower(services[j].Name)
		})
	case "name_desc":
		sort.Slice(services, func(i, j int) bool {
			return strings.ToLower(services[i].Name) > strings.ToLower(services[j].Name)
		})
	case "state":
		sort.Slice(services, func(i, j int) bool {
			return strings.ToLower(services[i].ActiveState) < strings.ToLower(services[j].ActiveState)
		})
	case "load_state":
		sort.Slice(services, func(i, j int) bool {
			return strings.ToLower(services[i].LoadState) < strings.ToLower(services[j].LoadState)
		})
	case "":
		// default sort by name_asc
		sort.Slice(services, func(i, j int) bool {
			return strings.ToLower(services[i].Name) < strings.ToLower(services[j].Name)
		})
	default:
		return nil, ErrInvalidSort
	}

	if limit > 0 && limit < len(services) {
		services = services[:limit]
	}

	return &dto.ServiceResponseDTO{
		CollectedAt: entry.CollectedAt,
		Services:    services,
	}, nil
}
