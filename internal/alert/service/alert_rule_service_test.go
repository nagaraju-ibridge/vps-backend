package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
	"vpsmonitoring-backend/internal/alert/dto"
	"vpsmonitoring-backend/internal/alert/models"
	alertRepository "vpsmonitoring-backend/internal/alert/repository"
	appModels "vpsmonitoring-backend/internal/application/models"
	appRepository "vpsmonitoring-backend/internal/application/repository"
	serverDTO "vpsmonitoring-backend/internal/server/dto"
	serverService "vpsmonitoring-backend/internal/server/service"
)

type ruleRepoMock struct {
	alertRepository.AlertRuleRepository
	rules map[int64]*models.AlertRule
	next  int64
}

func (m *ruleRepoMock) Create(_ context.Context, r *models.AlertRule) error {
	m.next++
	r.ID = m.next
	r.CreatedAt = time.Now().UTC()
	r.UpdatedAt = r.CreatedAt
	copy := *r
	m.rules[r.ID] = &copy
	return nil
}
func (m *ruleRepoMock) GetByIDAndUserID(_ context.Context, id, userID int64) (*models.AlertRule, error) {
	r, ok := m.rules[id]
	if !ok || r.UserID != userID {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *r
	return &copy, nil
}
func (m *ruleRepoMock) ListByUserID(_ context.Context, userID int64) ([]models.AlertRule, error) {
	var out []models.AlertRule
	for _, r := range m.rules {
		if r.UserID == userID {
			out = append(out, *r)
		}
	}
	return out, nil
}
func (m *ruleRepoMock) Update(_ context.Context, r *models.AlertRule) error {
	r.UpdatedAt = time.Now().UTC()
	copy := *r
	m.rules[r.ID] = &copy
	return nil
}
func (m *ruleRepoMock) Delete(_ context.Context, r *models.AlertRule) error {
	delete(m.rules, r.ID)
	return nil
}

type serverServiceMock struct {
	serverService.ServerService
	owners map[int64]int64
}

func (m *serverServiceMock) GetServer(_ context.Context, id, userID int64) (*serverDTO.ServerResponse, error) {
	if m.owners[id] != userID {
		return nil, serverService.ErrServerNotFound
	}
	return &serverDTO.ServerResponse{ID: id}, nil
}

type appRepoMock struct {
	appRepository.ApplicationRepository
	apps map[int64]*appModels.Application
}

func (m *appRepoMock) GetByID(_ context.Context, id int64) (*appModels.Application, error) {
	app, ok := m.apps[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return app, nil
}

func setupService() (AlertRuleService, *ruleRepoMock) {
	repo := &ruleRepoMock{rules: map[int64]*models.AlertRule{}}
	return NewAlertRuleService(repo, &serverServiceMock{owners: map[int64]int64{1: 10, 2: 20}}, &appRepoMock{apps: map[int64]*appModels.Application{7: {ID: 7, ServerID: 1}, 8: {ID: 8, ServerID: 2}}}), repo
}
func boolPtr(v bool) *bool    { return &v }
func int64Ptr(v int64) *int64 { return &v }

func TestCreateAlertRules(t *testing.T) {
	svc, _ := setupService()
	ctx := context.Background()
	t.Run("valid server rule", func(t *testing.T) {
		got, err := svc.Create(ctx, 10, dto.CreateAlertRuleRequest{Name: "Server CPU High", ScopeType: "SERVER", ServerID: 1, ConditionType: "SERVER_CPU", Operator: "GT", Threshold: "90", DurationSeconds: 300, Severity: "WARNING"})
		if err != nil {
			t.Fatal(err)
		}
		if !got.IsEnabled || got.ApplicationID != nil {
			t.Fatalf("unexpected response: %+v", got)
		}
	})
	t.Run("valid application rule", func(t *testing.T) {
		got, err := svc.Create(ctx, 10, dto.CreateAlertRuleRequest{Name: "Calculator Down", ScopeType: "APPLICATION", ServerID: 1, ApplicationID: int64Ptr(7), ConditionType: "APPLICATION_DOWN", Operator: "EQ", Threshold: "DOWN", DurationSeconds: 120, Severity: "CRITICAL", IsEnabled: boolPtr(false)})
		if err != nil {
			t.Fatal(err)
		}
		if got.IsEnabled || got.ApplicationID == nil || *got.ApplicationID != 7 {
			t.Fatalf("unexpected response: %+v", got)
		}
	})
}

func TestAlertRuleValidation(t *testing.T) {
	svc, _ := setupService()
	ctx := context.Background()
	base := dto.CreateAlertRuleRequest{Name: "Valid Rule", ScopeType: "SERVER", ServerID: 1, ConditionType: "SERVER_CPU", Operator: "GT", Threshold: "90", Severity: "WARNING"}
	tests := []struct {
		name   string
		mutate func(*dto.CreateAlertRuleRequest)
		want   error
	}{
		{"empty name", func(r *dto.CreateAlertRuleRequest) { r.Name = "" }, ErrInvalidName},
		{"invalid condition", func(r *dto.CreateAlertRuleRequest) { r.ConditionType = "NOPE" }, ErrInvalidCondition},
		{"invalid operator", func(r *dto.CreateAlertRuleRequest) { r.Operator = "LIKE" }, ErrInvalidOperator},
		{"invalid severity", func(r *dto.CreateAlertRuleRequest) { r.Severity = "HIGH" }, ErrInvalidSeverity},
		{"negative duration", func(r *dto.CreateAlertRuleRequest) { r.DurationSeconds = -1 }, ErrInvalidDuration},
		{"missing server", func(r *dto.CreateAlertRuleRequest) { r.ServerID = 0 }, ErrServerTarget},
		{"missing application", func(r *dto.CreateAlertRuleRequest) {
			r.ScopeType = "APPLICATION"
			r.ConditionType = "APPLICATION_DOWN"
			r.Operator = "EQ"
			r.Threshold = "DOWN"
		}, ErrApplicationNeeded},
		{"wrong scope condition", func(r *dto.CreateAlertRuleRequest) {
			r.ConditionType = "APPLICATION_DOWN"
			r.Operator = "EQ"
			r.Threshold = "DOWN"
		}, ErrInvalidCombination},
		{"invalid numeric threshold", func(r *dto.CreateAlertRuleRequest) { r.Threshold = "DROP TABLE" }, ErrInvalidThreshold},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := base
			tt.mutate(&req)
			_, err := svc.Create(ctx, 10, req)
			if !errors.Is(err, tt.want) {
				t.Fatalf("want %v, got %v", tt.want, err)
			}
		})
	}
}

func TestAlertRuleOwnershipAndCRUD(t *testing.T) {
	svc, _ := setupService()
	ctx := context.Background()
	if _, err := svc.Create(ctx, 10, dto.CreateAlertRuleRequest{Name: "Foreign server", ScopeType: "SERVER", ServerID: 2, ConditionType: "SERVER_CPU", Operator: "GT", Threshold: "80", Severity: "WARNING"}); !errors.Is(err, ErrServerTarget) {
		t.Fatalf("expected server ownership rejection, got %v", err)
	}
	if _, err := svc.Create(ctx, 10, dto.CreateAlertRuleRequest{Name: "Foreign app", ScopeType: "APPLICATION", ServerID: 1, ApplicationID: int64Ptr(8), ConditionType: "APPLICATION_DOWN", Operator: "EQ", Threshold: "DOWN", Severity: "CRITICAL"}); !errors.Is(err, ErrApplicationTarget) {
		t.Fatalf("expected app ownership rejection, got %v", err)
	}
	created, err := svc.Create(ctx, 10, dto.CreateAlertRuleRequest{Name: "Owned", ScopeType: "SERVER", ServerID: 1, ConditionType: "SERVER_CPU", Operator: "GT", Threshold: "80", Severity: "WARNING"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Get(ctx, 20, created.ID); !errors.Is(err, ErrRuleNotFound) {
		t.Fatalf("cross-user get: %v", err)
	}
	name := "Updated"
	updated, err := svc.Update(ctx, 10, created.ID, dto.UpdateAlertRuleRequest{Name: &name})
	if err != nil || updated.Name != name {
		t.Fatalf("update failed: %+v %v", updated, err)
	}
	if _, err = svc.Update(ctx, 20, created.ID, dto.UpdateAlertRuleRequest{Name: &name}); !errors.Is(err, ErrRuleNotFound) {
		t.Fatalf("cross-user update: %v", err)
	}
	disabled := false
	status, err := svc.UpdateStatus(ctx, 10, created.ID, dto.UpdateAlertRuleStatusRequest{IsEnabled: &disabled})
	if err != nil || status.IsEnabled {
		t.Fatalf("disable failed: %+v %v", status, err)
	}
	if err = svc.Delete(ctx, 20, created.ID); !errors.Is(err, ErrRuleNotFound) {
		t.Fatalf("cross-user delete: %v", err)
	}
	if err = svc.Delete(ctx, 10, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Get(ctx, 10, created.ID); !errors.Is(err, ErrRuleNotFound) {
		t.Fatalf("rule should be deleted, got %v", err)
	}
}
