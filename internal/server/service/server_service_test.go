package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"vpsmonitoring-backend/internal/server/dto"
	"vpsmonitoring-backend/internal/server/models"
	"vpsmonitoring-backend/internal/server/repository"
	"vpsmonitoring-backend/internal/server/service"
)

// mockServerRepository is an in-memory repository implementing repository.ServerRepository
type mockServerRepository struct {
	servers map[int64]*models.Server
	nextID  int64
}

func newMockServerRepository() *mockServerRepository {
	return &mockServerRepository{
		servers: make(map[int64]*models.Server),
		nextID:  1,
	}
}

func (m *mockServerRepository) AutoMigrate() error {
	return nil
}

func (m *mockServerRepository) Create(ctx context.Context, server *models.Server) error {
	server.ID = m.nextID
	m.nextID++
	server.CreatedAt = time.Now()
	server.UpdatedAt = time.Now()

	// store a copy
	saved := *server
	m.servers[server.ID] = &saved
	return nil
}

func (m *mockServerRepository) GetByID(ctx context.Context, id int64) (*models.Server, error) {
	srv, exists := m.servers[id]
	if !exists || srv.DeletedAt.Valid {
		return nil, nil
	}
	cp := *srv
	return &cp, nil
}

func (m *mockServerRepository) GetByIDAndUserID(ctx context.Context, id int64, userID int64) (*models.Server, error) {
	srv, exists := m.servers[id]
	if !exists || srv.DeletedAt.Valid || srv.UserID != userID {
		return nil, nil
	}
	cp := *srv
	return &cp, nil
}

func (m *mockServerRepository) GetByUserID(ctx context.Context, userID int64) ([]models.Server, error) {
	var result []models.Server
	for _, srv := range m.servers {
		if srv.UserID == userID && !srv.DeletedAt.Valid {
			result = append(result, *srv)
		}
	}
	return result, nil
}

func (m *mockServerRepository) Update(ctx context.Context, server *models.Server) error {
	srv, exists := m.servers[server.ID]
	if !exists || srv.DeletedAt.Valid {
		return errors.New("server not found")
	}
	server.UpdatedAt = time.Now()
	saved := *server
	m.servers[server.ID] = &saved
	return nil
}

func (m *mockServerRepository) Delete(ctx context.Context, id int64, userID int64) error {
	srv, exists := m.servers[id]
	if !exists || srv.DeletedAt.Valid || srv.UserID != userID {
		return gorm.ErrRecordNotFound
	}
	// Simulate soft delete
	srv.DeletedAt = gorm.DeletedAt{Time: time.Now(), Valid: true}
	return nil
}

var _ repository.ServerRepository = (*mockServerRepository)(nil)

func TestServerService_CreateServer(t *testing.T) {
	repo := newMockServerRepository()
	svc := service.NewServerService(repo)
	ctx := context.Background()

	// 1. Valid creation
	req := dto.CreateServerRequest{Name: "Production VPS"}
	res, err := svc.CreateServer(ctx, 10, req)
	if err != nil {
		t.Fatalf("unexpected error creating server: %v", err)
	}

	if res.ID != 1 {
		t.Errorf("expected server ID 1, got %d", res.ID)
	}
	if res.Name != "Production VPS" {
		t.Errorf("expected name 'Production VPS', got '%s'", res.Name)
	}
	if res.Status != models.StatusPending {
		t.Errorf("expected initial status 'PENDING', got '%s'", res.Status)
	}

	// 2. Empty name should fail
	_, err = svc.CreateServer(ctx, 10, dto.CreateServerRequest{Name: "  "})
	if !errors.Is(err, service.ErrInvalidServerName) {
		t.Errorf("expected ErrInvalidServerName, got %v", err)
	}

	// 3. Name too short
	_, err = svc.CreateServer(ctx, 10, dto.CreateServerRequest{Name: "A"})
	if !errors.Is(err, service.ErrNameLength) {
		t.Errorf("expected ErrNameLength for 1 char, got %v", err)
	}
}

func TestServerService_OwnershipSecurity_GetServer(t *testing.T) {
	repo := newMockServerRepository()
	svc := service.NewServerService(repo)
	ctx := context.Background()

	// User A (ID=10) creates Server 1
	res1, _ := svc.CreateServer(ctx, 10, dto.CreateServerRequest{Name: "UserA Server 1"})
	// User B (ID=20) creates Server 2
	res2, _ := svc.CreateServer(ctx, 20, dto.CreateServerRequest{Name: "UserB Server 2"})

	// User A gets their own Server 1 -> should succeed
	serverA, err := svc.GetServer(ctx, res1.ID, 10)
	if err != nil || serverA == nil {
		t.Fatalf("User A should be able to get their own server: %v", err)
	}
	if serverA.Name != "UserA Server 1" {
		t.Errorf("expected 'UserA Server 1', got '%s'", serverA.Name)
	}

	// User A tries to get User B's Server 2 -> MUST FAIL with ErrServerNotFound
	_, err = svc.GetServer(ctx, res2.ID, 10)
	if !errors.Is(err, service.ErrServerNotFound) {
		t.Errorf("expected ErrServerNotFound when accessing another user's server, got %v", err)
	}

	// User B gets their own Server 2 -> should succeed
	serverB, err := svc.GetServer(ctx, res2.ID, 20)
	if err != nil || serverB == nil {
		t.Fatalf("User B should be able to get their own server: %v", err)
	}
}

func TestServerService_OwnershipSecurity_UpdateServer(t *testing.T) {
	repo := newMockServerRepository()
	svc := service.NewServerService(repo)
	ctx := context.Background()

	// User A creates server
	resA, _ := svc.CreateServer(ctx, 10, dto.CreateServerRequest{Name: "Old Name"})

	// User B tries to update User A's server -> MUST FAIL
	_, err := svc.UpdateServer(ctx, resA.ID, 20, dto.UpdateServerRequest{Name: "Hacked Name"})
	if !errors.Is(err, service.ErrServerNotFound) {
		t.Errorf("expected ErrServerNotFound when User B tries to update User A's server, got %v", err)
	}

	// User A updates their own server -> MUST SUCCEED
	updated, err := svc.UpdateServer(ctx, resA.ID, 10, dto.UpdateServerRequest{Name: "New Valid Name"})
	if err != nil {
		t.Fatalf("expected update to succeed for owner: %v", err)
	}
	if updated.Name != "New Valid Name" {
		t.Errorf("expected name to be 'New Valid Name', got '%s'", updated.Name)
	}
}

func TestServerService_OwnershipSecurity_DeleteServer(t *testing.T) {
	repo := newMockServerRepository()
	svc := service.NewServerService(repo)
	ctx := context.Background()

	// User A creates server
	resA, _ := svc.CreateServer(ctx, 10, dto.CreateServerRequest{Name: "Server To Delete"})

	// User B tries to delete User A's server -> MUST FAIL
	err := svc.DeleteServer(ctx, resA.ID, 20)
	if !errors.Is(err, service.ErrServerNotFound) {
		t.Errorf("expected ErrServerNotFound when User B tries to delete User A's server, got %v", err)
	}

	// User A deletes their own server -> MUST SUCCEED (soft delete)
	err = svc.DeleteServer(ctx, resA.ID, 10)
	if err != nil {
		t.Fatalf("expected delete to succeed for owner: %v", err)
	}

	// Once soft-deleted, User A querying it should return ErrServerNotFound
	_, err = svc.GetServer(ctx, resA.ID, 10)
	if !errors.Is(err, service.ErrServerNotFound) {
		t.Errorf("expected ErrServerNotFound after soft delete, got %v", err)
	}

	// List servers for User A should be empty
	servers, err := svc.GetUserServers(ctx, 10)
	if err != nil {
		t.Fatalf("unexpected error listing servers: %v", err)
	}
	if len(servers) != 0 {
		t.Errorf("expected 0 servers after delete, got %d", len(servers))
	}
}

func TestServerService_GetUserServers(t *testing.T) {
	repo := newMockServerRepository()
	svc := service.NewServerService(repo)
	ctx := context.Background()

	// User A creates 2 servers
	_, _ = svc.CreateServer(ctx, 10, dto.CreateServerRequest{Name: "Server A1"})
	_, _ = svc.CreateServer(ctx, 10, dto.CreateServerRequest{Name: "Server A2"})

	// User B creates 1 server
	_, _ = svc.CreateServer(ctx, 20, dto.CreateServerRequest{Name: "Server B1"})

	// User A lists servers -> only 2 servers returned
	serversA, err := svc.GetUserServers(ctx, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(serversA) != 2 {
		t.Errorf("expected 2 servers for User A, got %d", len(serversA))
	}

	// User B lists servers -> only 1 server returned
	serversB, err := svc.GetUserServers(ctx, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(serversB) != 1 {
		t.Errorf("expected 1 server for User B, got %d", len(serversB))
	}

	// User C has no servers -> returns empty slice
	serversC, err := svc.GetUserServers(ctx, 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(serversC) != 0 {
		t.Errorf("expected 0 servers for User C, got %d", len(serversC))
	}
}
