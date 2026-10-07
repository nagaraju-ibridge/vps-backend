package service

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"vpsmonitoring-backend/internal/application/dto"
	"vpsmonitoring-backend/internal/application/models"
	"vpsmonitoring-backend/internal/application/repository"
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
		return s, nil
	}
	return nil, serverService.ErrServerNotFound
}

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}

	err = db.AutoMigrate(&models.Application{})
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	return db
}

func ptrStr(s string) *string {
	return &s
}

func ptrInt(i int) *int {
	return &i
}

func ptrBool(b bool) *bool {
	return &b
}

func TestApplicationService_Create(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewApplicationRepository(db)

	serverSvc := &mockServerService{
		servers: map[int64]*serverDto.ServerResponse{
			1: {ID: 1}, // Mocks a successful lookup for server 1
		},
	}

	svc := NewApplicationService(repo, serverSvc, db)
	ctx := context.Background()

	t.Run("valid systemd app creation", func(t *testing.T) {
		req := dto.ApplicationCreateRequest{
			Name:       "Nginx",
			MatchType:  models.MatchTypeSystemdUnit,
			MatchValue: "nginx.service",
		}
		res, err := svc.Create(ctx, 100, 1, req)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if res.Name != "Nginx" {
			t.Errorf("expected Nginx, got %s", res.Name)
		}
		if res.IsEnabled != true {
			t.Errorf("expected IsEnabled true")
		}
	})

	t.Run("server ownership validation", func(t *testing.T) {
		req := dto.ApplicationCreateRequest{
			Name:       "Test",
			MatchType:  models.MatchTypeSystemdUnit,
			MatchValue: "test.service",
		}
		_, err := svc.Create(ctx, 100, 2, req) // Server 2 not owned
		if err != ErrServerOwnership {
			t.Errorf("expected ErrServerOwnership, got %v", err)
		}
	})

	t.Run("duplicate identity rejection", func(t *testing.T) {
		req := dto.ApplicationCreateRequest{
			Name:       "Nginx Duplicate",
			MatchType:  models.MatchTypeSystemdUnit,
			MatchValue: "nginx.service",
		}
		_, err := svc.Create(ctx, 100, 1, req)
		if err != ErrDuplicateIdentity {
			t.Errorf("expected ErrDuplicateIdentity, got %v", err)
		}
	})

	t.Run("invalid match type", func(t *testing.T) {
		req := dto.ApplicationCreateRequest{
			Name:       "Test",
			MatchType:  "invalid_type",
			MatchValue: "test",
		}
		_, err := svc.Create(ctx, 100, 1, req)
		if err != ErrInvalidMatchType {
			t.Errorf("expected ErrInvalidMatchType, got %v", err)
		}
	})

	t.Run("invalid HTTP URL", func(t *testing.T) {
		req := dto.ApplicationCreateRequest{
			Name:           "Test",
			MatchType:      models.MatchTypeExePath,
			MatchValue:     "/usr/bin/test",
			MonitorHTTPURL: "not-a-url",
		}
		_, err := svc.Create(ctx, 100, 1, req)
		if err != ErrInvalidHTTPURL {
			t.Errorf("expected ErrInvalidHTTPURL, got %v", err)
		}
	})

	t.Run("invalid log source path mismatch", func(t *testing.T) {
		req := dto.ApplicationCreateRequest{
			Name:          "Test",
			MatchType:     models.MatchTypeExePath,
			MatchValue:    "/usr/bin/test",
			LogSourcePath: "/var/log/nginx/access.log", // provided path without type
		}
		_, err := svc.Create(ctx, 100, 1, req)
		if err != ErrLogSourcePathInvalid {
			t.Errorf("expected ErrLogSourcePathInvalid, got %v", err)
		}
	})
}

func TestApplicationService_Limits(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewApplicationRepository(db)

	serverSvc := &mockServerService{
		servers: map[int64]*serverDto.ServerResponse{
			1: {ID: 1},
		},
	}

	svc := NewApplicationService(repo, serverSvc, db)
	ctx := context.Background()

	t.Run("15 app limit enforcement", func(t *testing.T) {
		for i := 0; i < 15; i++ {
			err := db.Create(&models.Application{
				ServerID:   1,
				Name:       "App",
				MatchType:  models.MatchTypeExePath,
				MatchValue: string(rune(i)),
				IsEnabled:  true,
			}).Error
			if err != nil {
				t.Fatalf("failed to seed db")
			}
		}

		req := dto.ApplicationCreateRequest{
			Name:       "16th app",
			MatchType:  models.MatchTypeSystemdUnit,
			MatchValue: "16th.service",
		}
		_, err := svc.Create(ctx, 100, 1, req)
		if err != ErrEnabledLimitReached {
			t.Errorf("expected ErrEnabledLimitReached, got %v", err)
		}
	})
}

func TestApplicationService_UpdateAndDelete(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewApplicationRepository(db)

	serverSvc := &mockServerService{
		servers: map[int64]*serverDto.ServerResponse{
			1: {ID: 1},
			2: {ID: 2},
		},
	}

	svc := NewApplicationService(repo, serverSvc, db)
	ctx := context.Background()

	// Seed app
	app := &models.Application{
		ServerID:   1,
		Name:       "App1",
		MatchType:  models.MatchTypeSystemdUnit,
		MatchValue: "app1.service",
		IsEnabled:  true,
	}
	db.Create(app)

	t.Run("update config", func(t *testing.T) {
		req := dto.ApplicationUpdateRequest{
			Name:        ptrStr("App1 Updated"),
			MonitorPort: ptrInt(8080),
		}
		res, err := svc.Update(ctx, 100, 1, app.ID, req)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if res.Name != "App1 Updated" {
			t.Errorf("expected name to change, got %s", res.Name)
		}
		if res.MonitorPort == nil || *res.MonitorPort != 8080 {
			t.Errorf("expected port 8080")
		}
	})

	t.Run("delete app belonging to another server", func(t *testing.T) {
		err := svc.Delete(ctx, 100, 2, app.ID)
		if err != ErrApplicationNotFound {
			t.Errorf("expected ErrApplicationNotFound, got %v", err)
		}
	})

	t.Run("delete app", func(t *testing.T) {
		err := svc.Delete(ctx, 100, 1, app.ID)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		_, err = svc.Get(ctx, 100, 1, app.ID)
		if err != ErrApplicationNotFound {
			t.Errorf("expected ErrApplicationNotFound, got %v", err)
		}
	})
}

func TestApplicationService_GetAgentConfig(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewApplicationRepository(db)
	mockServerSvc := &mockServerService{}
	svc := NewApplicationService(repo, mockServerSvc, db)

	// Create enabled app
	_, _ = svc.Create(context.Background(), 1, 1, dto.ApplicationCreateRequest{
		Name: "Enabled App", MatchType: "systemd_unit", MatchValue: "enabled.service",
	})

	// Create disabled app
	res, _ := svc.Create(context.Background(), 1, 1, dto.ApplicationCreateRequest{
		Name: "Disabled App", MatchType: "systemd_unit", MatchValue: "disabled.service",
	})

	// Set it to disabled
	nameStr := "Disabled App"
	matchTypeStr := "systemd_unit"
	matchValueStr := "disabled.service"
	isEnab := false
	_, _ = svc.Update(context.Background(), 1, 1, res.ID, dto.ApplicationUpdateRequest{
		Name: &nameStr, MatchType: &matchTypeStr, MatchValue: &matchValueStr, IsEnabled: &isEnab,
	})

	// Get agent config
	apps, err := svc.GetAgentConfig(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(apps) != 1 {
		t.Fatalf("expected 1 enabled app, got %d", len(apps))
	}
	if apps[0].Name != "Enabled App" {
		t.Errorf("expected Enabled App, got %s", apps[0].Name)
	}
}
