package service

import (
	"context"
	"testing"
	"time"

	serverDto "vpsmonitoring-backend/internal/server/dto"
	"vpsmonitoring-backend/internal/systemd/cache"
	"vpsmonitoring-backend/internal/systemd/dto"
)

type mockServerService struct {
	server *serverDto.ServerResponse
	err    error
}

func (m *mockServerService) GetServer(ctx context.Context, id int64, userID int64) (*serverDto.ServerResponse, error) {
	return m.server, m.err
}

func (m *mockServerService) CreateServer(ctx context.Context, userID int64, req serverDto.CreateServerRequest) (*serverDto.ServerResponse, error) {
	return nil, nil
}
func (m *mockServerService) UpdateServer(ctx context.Context, id int64, userID int64, req serverDto.UpdateServerRequest) (*serverDto.ServerResponse, error) {
	return nil, nil
}
func (m *mockServerService) DeleteServer(ctx context.Context, id, userID int64) error { return nil }
func (m *mockServerService) GetUserServers(ctx context.Context, userID int64) ([]serverDto.ServerResponse, error) {
	return nil, nil
}

func TestSystemdService_IngestAndGet(t *testing.T) {
	c := cache.NewSystemdCache()
	mockSvc := &mockServerService{server: &serverDto.ServerResponse{ID: 1}}
	svc := NewSystemdService(c, mockSvc)

	now := time.Now()
	err := svc.IngestSnapshot(1, dto.ServicePayloadDTO{
		CollectedAt: now,
		Services: []dto.ServiceSnapshotDTO{
			{Name: "b.service", ActiveState: "active"},
			{Name: "a.service", ActiveState: "inactive"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test sort default
	resp, err := svc.GetLatestSnapshot(context.Background(), 1, 1, 50, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Services) != 2 {
		t.Fatalf("expected 2 services, got %d", len(resp.Services))
	}
	if resp.Services[0].Name != "a.service" {
		t.Errorf("expected a.service first, got %s", resp.Services[0].Name)
	}

	// Test sort name_desc
	resp, err = svc.GetLatestSnapshot(context.Background(), 1, 1, 50, "name_desc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Services[0].Name != "b.service" {
		t.Errorf("expected b.service first, got %s", resp.Services[0].Name)
	}

	// Test limit
	resp, err = svc.GetLatestSnapshot(context.Background(), 1, 1, 1, "name_asc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(resp.Services))
	}

	// Test no snapshot
	_, err = svc.GetLatestSnapshot(context.Background(), 999, 1, 50, "")
	if err != ErrNoSnapshot {
		t.Errorf("expected ErrNoSnapshot, got %v", err)
	}
}

func TestSystemdService_Invalid(t *testing.T) {
	c := cache.NewSystemdCache()
	mockSvc := &mockServerService{server: &serverDto.ServerResponse{ID: 1}}
	svc := NewSystemdService(c, mockSvc)

	// test limit 0
	_, err := svc.GetLatestSnapshot(context.Background(), 1, 1, 0, "")
	if err != ErrInvalidLimit {
		t.Errorf("expected ErrInvalidLimit, got %v", err)
	}

	// inject snapshot so we get past ErrNoSnapshot
	svc.IngestSnapshot(1, dto.ServicePayloadDTO{
		CollectedAt: time.Now(),
		Services:    []dto.ServiceSnapshotDTO{{Name: "test"}},
	})

	// test invalid sort
	_, err = svc.GetLatestSnapshot(context.Background(), 1, 1, 50, "invalid")
	if err != ErrInvalidSort {
		t.Errorf("expected ErrInvalidSort, got %v", err)
	}
}
