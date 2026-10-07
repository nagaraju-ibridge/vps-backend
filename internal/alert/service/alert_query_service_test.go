package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"vpsmonitoring-backend/internal/alert/dto"
	"vpsmonitoring-backend/internal/alert/models"
	"vpsmonitoring-backend/internal/alert/repository"
	appModels "vpsmonitoring-backend/internal/application/models"
)

type alertQueryRepoMock struct {
	repository.AlertRepository
	alerts    []models.Alert
	lastQuery repository.AlertQuery
}

func (m *alertQueryRepoMock) GetByIDAndUserID(_ context.Context, id uuid.UUID, userID int64) (*models.Alert, error) {
	for i := range m.alerts {
		if m.alerts[i].ID == id && m.alerts[i].UserID == userID {
			return &m.alerts[i], nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}
func (m *alertQueryRepoMock) List(_ context.Context, query repository.AlertQuery) ([]models.Alert, int64, error) {
	m.lastQuery = query
	filtered := make([]models.Alert, 0)
	for _, alert := range m.alerts {
		if alert.UserID != query.UserID {
			continue
		}
		if query.ServerID != nil && alert.ServerID != *query.ServerID {
			continue
		}
		if query.ApplicationID != nil && (alert.ApplicationID == nil || *alert.ApplicationID != *query.ApplicationID) {
			continue
		}
		if query.Status != "" && alert.Status != query.Status {
			continue
		}
		if query.Severity != "" && alert.Severity != query.Severity {
			continue
		}
		if query.ConditionType != "" && alert.ConditionType != query.ConditionType {
			continue
		}
		filtered = append(filtered, alert)
	}
	total := int64(len(filtered))
	start := query.Offset
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + query.Limit
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total, nil
}

func queryFixture() (*alertQueryService, *alertQueryRepoMock) {
	app7 := int64(7)
	now := time.Now().UTC()
	repo := &alertQueryRepoMock{alerts: []models.Alert{{ID: uuid.New(), UserID: 10, ServerID: 1, RuleID: 1, RuleName: "CPU", Severity: models.SeverityCritical, ConditionType: models.ConditionServerCPU, Status: models.AlertStatusActive, Message: "safe", FirstTriggeredAt: now, LastTriggeredAt: now, LastObservedState: "TRUE", CreatedAt: now, UpdatedAt: now}, {ID: uuid.New(), UserID: 10, ServerID: 1, ApplicationID: &app7, RuleID: 2, RuleName: "App", Severity: models.SeverityWarning, ConditionType: models.ConditionApplicationDown, Status: models.AlertStatusResolved, Message: "safe", FirstTriggeredAt: now, LastTriggeredAt: now, LastObservedState: "FALSE", CreatedAt: now.Add(-time.Minute), UpdatedAt: now}, {ID: uuid.New(), UserID: 20, ServerID: 2, RuleID: 3, RuleName: "Other", Severity: models.SeverityCritical, ConditionType: models.ConditionServerCPU, Status: models.AlertStatusActive, Message: "hidden", FirstTriggeredAt: now, LastTriggeredAt: now, LastObservedState: "TRUE", CreatedAt: now, UpdatedAt: now}}}
	service := NewAlertQueryService(repo, &serverServiceMock{owners: map[int64]int64{1: 10, 2: 20}}, &appRepoMock{apps: map[int64]*appModels.Application{7: {ID: 7, ServerID: 1}, 8: {ID: 8, ServerID: 2}}}).(*alertQueryService)
	return service, repo
}

func TestAlertQueryListFiltersPaginationAndSafety(t *testing.T) {
	svc, repo := queryFixture()
	filter := dto.AlertListFilter{Page: 1, Limit: 1, Status: "active", Severity: "critical", ConditionType: "server_cpu"}
	response, err := svc.List(context.Background(), 10, filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Alerts) != 1 || response.Total != 1 || response.Page != 1 || response.Limit != 1 || response.TotalPages != 1 {
		t.Fatalf("unexpected response: %+v", response)
	}
	if repo.lastQuery.Status != "ACTIVE" || repo.lastQuery.Severity != "CRITICAL" || repo.lastQuery.ConditionType != "SERVER_CPU" {
		t.Fatalf("filters not combined: %+v", repo.lastQuery)
	}
	encoded, _ := json.Marshal(response)
	for _, secret := range []string{"password", "access_token", "agent_credential", "installation_token", "raw_log"} {
		if stringContains(string(encoded), secret) {
			t.Fatalf("response leaked field %s: %s", secret, encoded)
		}
	}
	empty, err := svc.List(context.Background(), 99, dto.AlertListFilter{Page: 1, Limit: 50})
	if err != nil || empty.Alerts == nil || len(empty.Alerts) != 0 || empty.TotalPages != 0 {
		t.Fatalf("empty list: %+v %v", empty, err)
	}
}
func stringContains(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}

func TestAlertQueryValidation(t *testing.T) {
	svc, _ := queryFixture()
	base := dto.AlertListFilter{Page: 1, Limit: 50}
	tests := []struct {
		name   string
		change func(*dto.AlertListFilter)
		want   error
	}{{"page", func(f *dto.AlertListFilter) { f.Page = 0 }, ErrInvalidAlertPage}, {"limit zero", func(f *dto.AlertListFilter) { f.Limit = 0 }, ErrInvalidAlertLimit}, {"limit high", func(f *dto.AlertListFilter) { f.Limit = 101 }, ErrInvalidAlertLimit}, {"status", func(f *dto.AlertListFilter) { f.Status = "garbage" }, ErrInvalidAlertStatus}, {"severity", func(f *dto.AlertListFilter) { f.Severity = "garbage" }, ErrInvalidAlertSeverity}, {"condition", func(f *dto.AlertListFilter) { f.ConditionType = "garbage" }, ErrInvalidAlertCondition}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := base
			tt.change(&filter)
			_, err := svc.List(context.Background(), 10, filter)
			if !errors.Is(err, tt.want) {
				t.Fatalf("want %v got %v", tt.want, err)
			}
		})
	}
}

func TestAlertQueryOwnershipAndScopes(t *testing.T) {
	svc, repo := queryFixture()
	ctx := context.Background()
	own := repo.alerts[0].ID
	if _, err := svc.Get(ctx, 10, own); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, 20, own); !errors.Is(err, ErrAlertNotFound) {
		t.Fatalf("cross user get: %v", err)
	}
	if _, err := svc.ListByServer(ctx, 10, 2, dto.AlertListFilter{Page: 1, Limit: 50}); !errors.Is(err, ErrAlertResourceNotFound) {
		t.Fatalf("cross user server: %v", err)
	}
	serverAlerts, err := svc.ListByServer(ctx, 10, 1, dto.AlertListFilter{Page: 1, Limit: 50})
	if err != nil || len(serverAlerts.Alerts) != 2 {
		t.Fatalf("server list must include server and app alerts: %+v %v", serverAlerts, err)
	}
	if _, err := svc.ListByApplication(ctx, 10, 1, 8, dto.AlertListFilter{Page: 1, Limit: 50}); !errors.Is(err, ErrAlertResourceNotFound) {
		t.Fatalf("cross server app: %v", err)
	}
	appAlerts, err := svc.ListByApplication(ctx, 10, 1, 7, dto.AlertListFilter{Page: 1, Limit: 50})
	if err != nil || len(appAlerts.Alerts) != 1 || appAlerts.Alerts[0].ApplicationID == nil || *appAlerts.Alerts[0].ApplicationID != 7 {
		t.Fatalf("application scope: %+v %v", appAlerts, err)
	}
}
