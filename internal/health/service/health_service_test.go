package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"vpsmonitoring-backend/internal/health/dto"
	"vpsmonitoring-backend/internal/health/models"
	serverDto "vpsmonitoring-backend/internal/server/dto"
	serverService "vpsmonitoring-backend/internal/server/service"
)

// --- Mock ServerService ---
type mockServerService struct {
	serverService.ServerService
	servers map[int64]*serverDto.ServerResponse
}

func (m *mockServerService) GetServer(ctx context.Context, id int64, userID int64) (*serverDto.ServerResponse, error) {
	if s, ok := m.servers[id]; ok {
		// Mock logic assumes if it exists in map, it's valid for this mock.
		// We'll trust the test setup.
		return s, nil
	}
	return nil, ErrServerNotFound
}

// --- Mock HealthRepository ---
type mockHealthRepository struct {
	configs []models.HttpHealthConfig
	err     error
}

func (m *mockHealthRepository) Create(config *models.HttpHealthConfig) error {
	if m.err != nil {
		return m.err
	}
	config.ID = int64(len(m.configs) + 1)
	config.CreatedAt = time.Now()
	config.UpdatedAt = time.Now()
	m.configs = append(m.configs, *config)
	return nil
}

func (m *mockHealthRepository) GetByID(id int64) (*models.HttpHealthConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	for _, c := range m.configs {
		if c.ID == id {
			return &c, nil
		}
	}
	return nil, ErrConfigNotFound
}

func (m *mockHealthRepository) ListByServerID(serverID int64) ([]models.HttpHealthConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	var res []models.HttpHealthConfig
	for _, c := range m.configs {
		if c.ServerID == serverID {
			res = append(res, c)
		}
	}
	return res, nil
}

func (m *mockHealthRepository) ListActiveByServerID(serverID int64) ([]models.HttpHealthConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	var res []models.HttpHealthConfig
	for _, c := range m.configs {
		if c.ServerID == serverID && c.IsActive {
			res = append(res, c)
		}
	}
	return res, nil
}

func (m *mockHealthRepository) Delete(id int64) error {
	if m.err != nil {
		return m.err
	}
	for i, c := range m.configs {
		if c.ID == id {
			m.configs = append(m.configs[:i], m.configs[i+1:]...)
			return nil
		}
	}
	return ErrConfigNotFound
}

func (m *mockHealthRepository) CountByServerID(serverID int64) (int64, error) {
	if m.err != nil {
		return 0, m.err
	}
	count := int64(0)
	for _, c := range m.configs {
		if c.ServerID == serverID {
			count++
		}
	}
	return count, nil
}

func (m *mockHealthRepository) AutoMigrate() error {
	return nil
}

func TestHealthService_CreateConfig(t *testing.T) {
	serverSvc := &mockServerService{
		servers: map[int64]*serverDto.ServerResponse{
			1: {ID: 1}, // Mocks a successful lookup
			// Server 2 doesn't exist in map so it returns ErrServerNotFound
		},
	}
	repo := &mockHealthRepository{}
	cache := NewHealthResultCache()
	svc := NewHealthService(repo, serverSvc, cache)

	ctx := context.Background()

	t.Run("valid config creation", func(t *testing.T) {
		req := dto.CreateHealthConfigRequest{URL: "https://example.com/health"}
		res, err := svc.CreateConfig(ctx, 1, 100, req)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if res.URL != req.URL {
			t.Errorf("expected URL %s, got %s", req.URL, res.URL)
		}
		if res.IntervalSec != DefaultIntervalSec {
			t.Errorf("expected default interval %d, got %d", DefaultIntervalSec, res.IntervalSec)
		}
		if res.IsActive != true {
			t.Errorf("expected active true, got false")
		}
	})

	t.Run("invalid URL", func(t *testing.T) {
		req := dto.CreateHealthConfigRequest{URL: "not-a-url"}
		_, err := svc.CreateConfig(ctx, 1, 100, req)
		if err != ErrInvalidURL {
			t.Errorf("expected ErrInvalidURL, got %v", err)
		}
	})

	t.Run("unsupported scheme", func(t *testing.T) {
		req := dto.CreateHealthConfigRequest{URL: "ftp://example.com"}
		_, err := svc.CreateConfig(ctx, 1, 100, req)
		if err != ErrUnsupportedScheme {
			t.Errorf("expected ErrUnsupportedScheme, got %v", err)
		}
	})

	t.Run("missing hostname", func(t *testing.T) {
		req := dto.CreateHealthConfigRequest{URL: "http:///"}
		_, err := svc.CreateConfig(ctx, 1, 100, req)
		if err != ErrMissingHostname {
			t.Errorf("expected ErrMissingHostname, got %v", err)
		}
	})

	t.Run("interval below minimum", func(t *testing.T) {
		val := 5
		req := dto.CreateHealthConfigRequest{URL: "http://example.com", IntervalSec: &val}
		_, err := svc.CreateConfig(ctx, 1, 100, req)
		if err != ErrInvalidInterval {
			t.Errorf("expected ErrInvalidInterval, got %v", err)
		}
	})

	t.Run("interval above maximum", func(t *testing.T) {
		val := 5000
		req := dto.CreateHealthConfigRequest{URL: "http://example.com", IntervalSec: &val}
		_, err := svc.CreateConfig(ctx, 1, 100, req)
		if err != ErrInvalidInterval {
			t.Errorf("expected ErrInvalidInterval, got %v", err)
		}
	})

	t.Run("server belonging to another user", func(t *testing.T) {
		req := dto.CreateHealthConfigRequest{URL: "http://example.com"}
		_, err := svc.CreateConfig(ctx, 2, 100, req) // server 2 belongs to user 200
		if err != ErrServerNotFound {
			t.Errorf("expected ErrServerNotFound, got %v", err)
		}
	})

	t.Run("10-check limit", func(t *testing.T) {
		// Fill up to 10
		repo.configs = nil
		for i := 0; i < 10; i++ {
			repo.Create(&models.HttpHealthConfig{ServerID: 1})
		}
		req := dto.CreateHealthConfigRequest{URL: "http://example.com"}
		_, err := svc.CreateConfig(ctx, 1, 100, req)
		if err != ErrTooManyChecks {
			t.Errorf("expected ErrTooManyChecks, got %v", err)
		}
	})

	t.Run("database/repository error handling", func(t *testing.T) {
		repo.configs = nil
		repo.err = errors.New("db down")
		req := dto.CreateHealthConfigRequest{URL: "http://example.com"}
		_, err := svc.CreateConfig(ctx, 1, 100, req)
		if err == nil || err.Error() != "db down" {
			t.Errorf("expected db error, got %v", err)
		}
		repo.err = nil
	})
}

func TestHealthService_ListAndDelete(t *testing.T) {
	serverSvc := &mockServerService{
		servers: map[int64]*serverDto.ServerResponse{
			1: {ID: 1},
			2: {ID: 2},
		},
	}
	repo := &mockHealthRepository{
		configs: []models.HttpHealthConfig{
			{ID: 1, ServerID: 1, URL: "http://s1.com", IsActive: true},
			{ID: 2, ServerID: 2, URL: "http://s2.com", IsActive: true},
		},
	}
	cache := NewHealthResultCache()
	svc := NewHealthService(repo, serverSvc, cache)
	ctx := context.Background()

	t.Run("successful listing", func(t *testing.T) {
		res, err := svc.ListConfigs(ctx, 1, 100)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 || res[0].ID != 1 {
			t.Errorf("expected config 1, got %v", res)
		}
	})

	t.Run("successful deletion", func(t *testing.T) {
		err := svc.DeleteConfig(ctx, 1, 100, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		res, _ := svc.ListConfigs(ctx, 1, 100)
		if len(res) != 0 {
			t.Errorf("expected empty list after deletion")
		}
	})

	t.Run("config belonging to another server", func(t *testing.T) {
		// Config 2 belongs to Server 2
		err := svc.DeleteConfig(ctx, 1, 100, 2)
		if err != ErrConfigNotFound {
			t.Errorf("expected ErrConfigNotFound, got %v", err)
		}
	})

	t.Run("get agent config", func(t *testing.T) {
		repo.configs = []models.HttpHealthConfig{
			{ID: 1, ServerID: 1, URL: "http://active.com", IsActive: true},
			{ID: 2, ServerID: 1, URL: "http://inactive.com", IsActive: false},
			{ID: 3, ServerID: 2, URL: "http://other.com", IsActive: true},
		}

		res, err := svc.GetAgentConfig(ctx, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res) != 1 || res[0].ID != 1 {
			t.Errorf("expected only active config for server 1, got %v", res)
		}
	})

	t.Run("ingest health checks", func(t *testing.T) {
		repo.configs = []models.HttpHealthConfig{
			{ID: 1, ServerID: 1, URL: "http://active.com", IsActive: true},
		}

		now := time.Now()
		payload := dto.AgentHealthCheckPayload{
			CollectedAt: now,
			Checks: []dto.AgentHealthCheckResultDTO{
				{ConfigID: 1, URL: "http://active.com", StatusCode: 200, LatencyMs: 50, IsAvailable: true},
			},
		}

		err := svc.IngestHealthChecks(ctx, 1, payload)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		results, err := svc.GetHealthChecks(ctx, 1, 100)
		if err != nil {
			t.Fatalf("unexpected error getting checks: %v", err)
		}
		if len(results) != 1 || results[0].ConfigID != 1 {
			t.Errorf("expected 1 result, got %v", results)
		}
		if results[0].LatencyMs != 50 {
			t.Errorf("expected 50ms latency, got %d", results[0].LatencyMs)
		}
	})
}
