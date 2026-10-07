package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"vpsmonitoring-backend/internal/alert/evaluator"
	"vpsmonitoring-backend/internal/alert/models"
	"vpsmonitoring-backend/internal/alert/repository"
)

var ErrInvalidEvaluationIdentity = errors.New("invalid alert evaluation resource identity")

type AlertPersistenceService struct{ repo repository.AlertRepository }

func NewAlertPersistenceService(repo repository.AlertRepository) *AlertPersistenceService {
	return &AlertPersistenceService{repo: repo}
}

func numericCondition(condition string) bool {
	switch condition {
	case models.ConditionServerCPU, models.ConditionServerMemory, models.ConditionServerDisk, models.ConditionHTTPResponseTime, models.ConditionHTTP4XX, models.ConditionHTTP5XX:
		return true
	}
	return false
}

func observedValue(rule models.AlertRule, result evaluator.RuleEvaluationResult) *float64 {
	if !numericCondition(rule.ConditionType) || result.CurrentValue == "" {
		return nil
	}
	value, err := strconv.ParseFloat(result.CurrentValue, 64)
	if err != nil {
		return nil
	}
	return &value
}

func alertMessage(rule models.AlertRule, result evaluator.RuleEvaluationResult) string {
	value := result.CurrentValue
	switch rule.ConditionType {
	case models.ConditionServerCPU:
		return fmt.Sprintf("Server %d CPU usage is %s%%; rule threshold is %s.", rule.ServerID, value, rule.Threshold)
	case models.ConditionServerMemory:
		return fmt.Sprintf("Server %d memory usage is %s%%; rule threshold is %s.", rule.ServerID, value, rule.Threshold)
	case models.ConditionServerDisk:
		return fmt.Sprintf("Server %d disk usage is %s%%; rule threshold is %s.", rule.ServerID, value, rule.Threshold)
	case models.ConditionServerOffline:
		return fmt.Sprintf("Server %d is OFFLINE.", rule.ServerID)
	case models.ConditionApplicationDown:
		return fmt.Sprintf("Application %d is DOWN.", *rule.ApplicationID)
	case models.ConditionApplicationDegrade:
		return fmt.Sprintf("Application %d is DEGRADED.", *rule.ApplicationID)
	case models.ConditionHTTPHealthFailure:
		return fmt.Sprintf("HTTP health check failed for application %d.", *rule.ApplicationID)
	case models.ConditionHTTPResponseTime:
		return fmt.Sprintf("Application %d HTTP response time is %s ms; rule threshold is %s ms.", *rule.ApplicationID, value, rule.Threshold)
	case models.ConditionHTTP4XX:
		return fmt.Sprintf("Application %d observed %s HTTP 4xx responses; rule threshold is %s.", *rule.ApplicationID, value, rule.Threshold)
	case models.ConditionHTTP5XX:
		return fmt.Sprintf("Application %d observed %s HTTP 5xx responses; rule threshold is %s.", *rule.ApplicationID, value, rule.Threshold)
	case models.ConditionApplicationRestart:
		return fmt.Sprintf("Application %d restarted.", *rule.ApplicationID)
	default:
		return fmt.Sprintf("Alert rule %d condition is satisfied.", rule.ID)
	}
}

func validIdentity(rule models.AlertRule, result evaluator.RuleEvaluationResult) bool {
	if result.RuleID != rule.ID || rule.ID <= 0 || rule.UserID <= 0 || rule.ServerID <= 0 {
		return false
	}
	if rule.ScopeType == models.ScopeServer {
		return rule.ApplicationID == nil
	}
	return rule.ScopeType == models.ScopeApplication && rule.ApplicationID != nil && *rule.ApplicationID > 0
}

func (s *AlertPersistenceService) Persist(ctx context.Context, rule models.AlertRule, result evaluator.RuleEvaluationResult) error {
	if !validIdentity(rule, result) {
		return ErrInvalidEvaluationIdentity
	}
	if !rule.IsEnabled {
		_, err := s.repo.ResolveActive(ctx, rule.ID, rule.ServerID, rule.ApplicationID, result.EvaluatedAt, "FALSE", models.ResolutionRuleDisabled)
		return err
	}
	switch result.State {
	case evaluator.StateUnknown:
		return nil
	case evaluator.StateFalse:
		resolved, err := s.repo.ResolveActive(ctx, rule.ID, rule.ServerID, rule.ApplicationID, result.EvaluatedAt, string(evaluator.StateFalse), models.ResolutionConditionCleared)
		if err == nil && resolved {
			log.Printf("[INFO] alert resolved rule_id=%d server_id=%d", rule.ID, rule.ServerID)
		}
		return err
	case evaluator.StateTrue:
		if !result.DurationSatisfied {
			return nil
		}
		at := result.EvaluatedAt
		if at.IsZero() {
			at = time.Now().UTC()
		}
		alert := &models.Alert{RuleID: rule.ID, UserID: rule.UserID, ServerID: rule.ServerID, ApplicationID: rule.ApplicationID, RuleName: rule.Name, ConditionType: rule.ConditionType, Severity: rule.Severity, Status: models.AlertStatusActive, Message: alertMessage(rule, result), FirstTriggeredAt: at, LastTriggeredAt: at, LastObservedValue: observedValue(rule, result), LastObservedState: string(evaluator.StateTrue), CreatedAt: at, UpdatedAt: at}
		_, created, err := s.repo.CreateOrUpdateActive(ctx, alert)
		if errors.Is(err, repository.ErrAlertRuleInactive) {
			return nil
		}
		if err == nil && created {
			log.Printf("[INFO] alert created rule_id=%d server_id=%d", rule.ID, rule.ServerID)
		}
		return err
	default:
		return nil
	}
}

func (s *AlertPersistenceService) Reconcile(ctx context.Context, at time.Time) error {
	return s.repo.ResolveInactiveRules(ctx, at)
}
