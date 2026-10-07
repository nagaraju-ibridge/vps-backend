package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"vpsmonitoring-backend/internal/alert/evaluator"
	"vpsmonitoring-backend/internal/alert/models"
	"vpsmonitoring-backend/internal/alert/repository"
)

type alertRepoMock struct {
	repository.AlertRepository
	mu         sync.Mutex
	alerts     []*models.Alert
	reconciled bool
}

func sameResource(alert *models.Alert, ruleID, serverID int64, appID *int64) bool {
	if alert.RuleID != ruleID || alert.ServerID != serverID {
		return false
	}
	if alert.ApplicationID == nil || appID == nil {
		return alert.ApplicationID == nil && appID == nil
	}
	return *alert.ApplicationID == *appID
}
func cloneAlert(alert *models.Alert) *models.Alert {
	copy := *alert
	if alert.ApplicationID != nil {
		v := *alert.ApplicationID
		copy.ApplicationID = &v
	}
	if alert.LastObservedValue != nil {
		v := *alert.LastObservedValue
		copy.LastObservedValue = &v
	}
	return &copy
}
func (m *alertRepoMock) CreateOrUpdateActive(_ context.Context, in *models.Alert) (*models.Alert, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, alert := range m.alerts {
		if alert.Status == models.AlertStatusActive && sameResource(alert, in.RuleID, in.ServerID, in.ApplicationID) {
			alert.LastTriggeredAt = in.LastTriggeredAt
			alert.LastObservedValue = in.LastObservedValue
			alert.LastObservedState = in.LastObservedState
			alert.Message = in.Message
			alert.UpdatedAt = in.UpdatedAt
			return cloneAlert(alert), false, nil
		}
	}
	copy := cloneAlert(in)
	copy.ID = uuid.New()
	m.alerts = append(m.alerts, copy)
	return cloneAlert(copy), true, nil
}
func (m *alertRepoMock) ResolveActive(_ context.Context, ruleID, serverID int64, appID *int64, at time.Time, state, reason string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, alert := range m.alerts {
		if alert.Status == models.AlertStatusActive && sameResource(alert, ruleID, serverID, appID) {
			alert.Status = models.AlertStatusResolved
			alert.ResolvedAt = &at
			alert.LastObservedState = state
			alert.ResolutionReason = &reason
			alert.UpdatedAt = at
			return true, nil
		}
	}
	return false, nil
}
func (m *alertRepoMock) ResolveInactiveRules(context.Context, time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reconciled = true
	return nil
}
func (m *alertRepoMock) count(status string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, alert := range m.alerts {
		if alert.Status == status {
			count++
		}
	}
	return count
}
func applicationID(v int64) *int64 { return &v }

func persistenceRule(condition string, application bool) models.AlertRule {
	rule := models.AlertRule{ID: 12, UserID: 3, ServerID: 5, Name: "Test rule", ScopeType: models.ScopeServer, ConditionType: condition, Operator: models.OperatorGT, Threshold: "80", Severity: models.SeverityCritical, IsEnabled: true}
	if application {
		rule.ScopeType = models.ScopeApplication
		rule.ApplicationID = applicationID(7)
	}
	return rule
}
func evaluation(state evaluator.ConditionState, at time.Time, value string, satisfied bool) evaluator.RuleEvaluationResult {
	return evaluator.RuleEvaluationResult{RuleID: 12, State: state, DurationSatisfied: satisfied, EvaluatedAt: at, CurrentValue: value}
}

func TestAlertPersistenceLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := &alertRepoMock{}
	svc := NewAlertPersistenceService(repo)
	rule := persistenceRule(models.ConditionServerCPU, false)
	at := time.Now().UTC()
	if err := svc.Persist(ctx, rule, evaluation(evaluator.StateTrue, at, "92.4", true)); err != nil {
		t.Fatal(err)
	}
	if repo.count(models.AlertStatusActive) != 1 {
		t.Fatal("qualifying true must create one active alert")
	}
	first := repo.alerts[0].FirstTriggeredAt
	later := at.Add(time.Minute)
	if err := svc.Persist(ctx, rule, evaluation(evaluator.StateTrue, later, "94.1", true)); err != nil {
		t.Fatal(err)
	}
	if len(repo.alerts) != 1 || !repo.alerts[0].FirstTriggeredAt.Equal(first) || !repo.alerts[0].LastTriggeredAt.Equal(later) {
		t.Fatal("repeated true must update without duplicating or resetting first trigger")
	}
	if repo.alerts[0].LastObservedValue == nil || *repo.alerts[0].LastObservedValue != 94.1 {
		t.Fatal("numeric observation not updated")
	}
	unknownAt := later.Add(time.Minute)
	if err := svc.Persist(ctx, rule, evaluation(evaluator.StateUnknown, unknownAt, "", false)); err != nil {
		t.Fatal(err)
	}
	if repo.count(models.AlertStatusActive) != 1 {
		t.Fatal("unknown must preserve active alert")
	}
	resolvedAt := unknownAt.Add(time.Minute)
	if err := svc.Persist(ctx, rule, evaluation(evaluator.StateFalse, resolvedAt, "70", false)); err != nil {
		t.Fatal(err)
	}
	if repo.count(models.AlertStatusResolved) != 1 || repo.alerts[0].ResolvedAt == nil || !repo.alerts[0].ResolvedAt.Equal(resolvedAt) {
		t.Fatal("false must resolve and set resolved_at")
	}
	reopenedAt := resolvedAt.Add(time.Minute)
	if err := svc.Persist(ctx, rule, evaluation(evaluator.StateTrue, reopenedAt, "93", true)); err != nil {
		t.Fatal(err)
	}
	if len(repo.alerts) != 2 || repo.count(models.AlertStatusActive) != 1 || repo.count(models.AlertStatusResolved) != 1 {
		t.Fatal("true after resolved must create a new active alert")
	}
}

func TestNoCreateCasesAndDisabledRule(t *testing.T) {
	ctx := context.Background()
	repo := &alertRepoMock{}
	svc := NewAlertPersistenceService(repo)
	rule := persistenceRule(models.ConditionServerCPU, false)
	at := time.Now().UTC()
	for _, result := range []evaluator.RuleEvaluationResult{evaluation(evaluator.StateTrue, at, "95", false), evaluation(evaluator.StateFalse, at, "20", false), evaluation(evaluator.StateUnknown, at, "", false)} {
		if err := svc.Persist(ctx, rule, result); err != nil {
			t.Fatal(err)
		}
	}
	if len(repo.alerts) != 0 {
		t.Fatal("non-qualified/false/unknown must not create")
	}
	if err := svc.Persist(ctx, rule, evaluation(evaluator.StateTrue, at, "95", true)); err != nil {
		t.Fatal(err)
	}
	rule.IsEnabled = false
	if err := svc.Persist(ctx, rule, evaluation(evaluator.StateTrue, at.Add(time.Minute), "96", true)); err != nil {
		t.Fatal(err)
	}
	if repo.count(models.AlertStatusActive) != 0 || repo.alerts[0].ResolutionReason == nil || *repo.alerts[0].ResolutionReason != models.ResolutionRuleDisabled {
		t.Fatal("disabled rule must resolve active alert")
	}
	if err := svc.Reconcile(ctx, at); err != nil || !repo.reconciled {
		t.Fatal("inactive rule reconciliation was not invoked")
	}
	if len(repo.alerts) != 1 {
		t.Fatal("reconciliation must preserve history")
	}
}

func TestAlertIdentityStateValuesAndMessages(t *testing.T) {
	ctx := context.Background()
	conditions := []struct {
		condition, value     string
		application, numeric bool
	}{{models.ConditionServerCPU, "91.2", false, true}, {models.ConditionServerMemory, "90", false, true}, {models.ConditionServerDisk, "88.7", false, true}, {models.ConditionServerOffline, "OFFLINE", false, false}, {models.ConditionApplicationDown, "DOWN", true, false}, {models.ConditionApplicationDegrade, "DEGRADED", true, false}, {models.ConditionHTTPHealthFailure, "DOWN", true, false}, {models.ConditionHTTPResponseTime, "1850", true, true}, {models.ConditionHTTP4XX, "42", true, true}, {models.ConditionHTTP5XX, "12", true, true}, {models.ConditionApplicationRestart, "true", true, false}}
	for index, tc := range conditions {
		t.Run(tc.condition, func(t *testing.T) {
			repo := &alertRepoMock{}
			svc := NewAlertPersistenceService(repo)
			rule := persistenceRule(tc.condition, tc.application)
			rule.ID = int64(index + 1)
			result := evaluation(evaluator.StateTrue, time.Now().UTC(), tc.value, true)
			result.RuleID = rule.ID
			if err := svc.Persist(ctx, rule, result); err != nil {
				t.Fatal(err)
			}
			alert := repo.alerts[0]
			if alert.UserID != rule.UserID || alert.ServerID != rule.ServerID || (tc.application && alert.ApplicationID == nil) || (!tc.application && alert.ApplicationID != nil) {
				t.Fatalf("incorrect ownership/resource identity: %+v", alert)
			}
			if alert.Message == "" {
				t.Fatal("message is empty")
			}
			if tc.numeric != (alert.LastObservedValue != nil) {
				t.Fatalf("numeric value mismatch: %+v", alert.LastObservedValue)
			}
		})
	}
}

func TestConcurrentQualifiedResultsDeduplicate(t *testing.T) {
	ctx := context.Background()
	repo := &alertRepoMock{}
	svc := NewAlertPersistenceService(repo)
	rule := persistenceRule(models.ConditionServerCPU, false)
	at := time.Now().UTC()
	var wait sync.WaitGroup
	for i := 0; i < 20; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := svc.Persist(ctx, rule, evaluation(evaluator.StateTrue, at, "99", true)); err != nil {
				t.Errorf("persist: %v", err)
			}
		}()
	}
	wait.Wait()
	if len(repo.alerts) != 1 {
		t.Fatalf("expected one active alert, got %d", len(repo.alerts))
	}
}

func TestInvalidEvaluationIdentity(t *testing.T) {
	svc := NewAlertPersistenceService(&alertRepoMock{})
	rule := persistenceRule(models.ConditionServerCPU, false)
	result := evaluation(evaluator.StateTrue, time.Now().UTC(), "90", true)
	result.RuleID = 999
	if err := svc.Persist(context.Background(), rule, result); err != ErrInvalidEvaluationIdentity {
		t.Fatalf("expected identity error, got %v", err)
	}
}
