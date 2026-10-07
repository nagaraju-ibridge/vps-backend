package evaluator

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"vpsmonitoring-backend/internal/alert/models"
)

const (
	DefaultInterval = 30 * time.Second
	TelemetryMaxAge = 3 * time.Minute
)

type pendingState struct{ StartedAt, LastEvaluatedAt time.Time }

type Evaluator struct {
	rules       RuleSource
	provider    SnapshotProvider
	interval    time.Duration
	now         func() time.Time
	mu          sync.RWMutex
	pending     map[int64]pendingState
	restartSeen map[int64]int64
	results     map[int64]RuleEvaluationResult
	sink        ResultSink
	running     atomic.Bool
}

func New(rules RuleSource, provider SnapshotProvider, interval time.Duration) *Evaluator {
	if interval <= 0 {
		interval = DefaultInterval
	}
	return &Evaluator{rules: rules, provider: provider, interval: interval, now: func() time.Time { return time.Now().UTC() }, pending: map[int64]pendingState{}, restartSeen: map[int64]int64{}, results: map[int64]RuleEvaluationResult{}}
}

func (e *Evaluator) SetResultSink(sink ResultSink) { e.mu.Lock(); defer e.mu.Unlock(); e.sink = sink }

func compareNumeric(value, threshold float64, operator string) bool {
	switch operator {
	case models.OperatorGT:
		return value > threshold
	case models.OperatorGTE:
		return value >= threshold
	case models.OperatorLT:
		return value < threshold
	case models.OperatorLTE:
		return value <= threshold
	case models.OperatorEQ:
		return value == threshold
	case models.OperatorNEQ:
		return value != threshold
	}
	return false
}
func compareState(value, threshold, operator string) bool {
	equal := strings.EqualFold(value, threshold)
	if operator == models.OperatorEQ {
		return equal
	}
	if operator == models.OperatorNEQ {
		return !equal
	}
	return false
}
func fresh(collected, timeNow time.Time) bool {
	return !collected.IsZero() && !collected.After(timeNow.Add(5*time.Minute)) && timeNow.Sub(collected) <= TelemetryMaxAge
}

func evaluateCondition(rule models.AlertRule, snapshot *Snapshot, now time.Time, restartSeen map[int64]int64) (ConditionState, string) {
	threshold, _ := strconv.ParseFloat(rule.Threshold, 64)
	if rule.ScopeType == models.ScopeServer {
		state, ok := snapshot.Servers[rule.ServerID]
		if !ok || !state.Exists {
			return StateUnknown, ""
		}
		if rule.ConditionType == models.ConditionServerOffline {
			if state.AgentStatus != "ONLINE" && state.AgentStatus != "OFFLINE" {
				return StateUnknown, state.AgentStatus
			}
			return condition(compareState(state.AgentStatus, rule.Threshold, rule.Operator)), state.AgentStatus
		}
		if state.Metric == nil || !fresh(state.Metric.CollectedAt, now) {
			return StateUnknown, ""
		}
		var value float64
		switch rule.ConditionType {
		case models.ConditionServerCPU:
			value = state.Metric.CPUUsagePercent
		case models.ConditionServerMemory:
			value = state.Metric.MemoryUsagePercent
		case models.ConditionServerDisk:
			if len(state.Metric.Disks) != 1 {
				return StateUnknown, ""
			}
			value = state.Metric.Disks[0].UsagePercent
		default:
			return StateUnknown, ""
		}
		return condition(compareNumeric(value, threshold, rule.Operator)), strconv.FormatFloat(value, 'f', -1, 64)
	}
	if rule.ApplicationID == nil {
		return StateUnknown, ""
	}
	state, ok := snapshot.Applications[*rule.ApplicationID]
	if !ok || !state.Exists {
		return StateUnknown, ""
	}
	if rule.ConditionType == models.ConditionApplicationRestart {
		previous, initialized := restartSeen[rule.ID]
		restartSeen[rule.ID] = state.LatestRestartID
		if !initialized || state.LatestRestartID == 0 {
			return StateFalse, "false"
		}
		detected := state.LatestRestartID > previous
		return condition(compareState(strconv.FormatBool(detected), rule.Threshold, rule.Operator)), strconv.FormatBool(detected)
	}
	if state.Metric == nil || !fresh(state.Metric.CollectedAt, now) {
		return StateUnknown, ""
	}
	metric := state.Metric
	switch rule.ConditionType {
	case models.ConditionApplicationDown, models.ConditionApplicationDegrade:
		if metric.Status == "" || metric.Status == "UNKNOWN" {
			return StateUnknown, metric.Status
		}
		return condition(compareState(metric.Status, rule.Threshold, rule.Operator)), metric.Status
	case models.ConditionHTTPHealthFailure:
		if !metric.HTTPConfigured || metric.HTTPAvailable == nil {
			return StateUnknown, ""
		}
		value := "UP"
		if !*metric.HTTPAvailable {
			value = "DOWN"
		}
		return condition(compareState(value, rule.Threshold, rule.Operator)), value
	case models.ConditionHTTPResponseTime:
		if metric.HTTPLatencyMs == nil {
			return StateUnknown, ""
		}
		value := float64(*metric.HTTPLatencyMs)
		return condition(compareNumeric(value, threshold, rule.Operator)), strconv.FormatInt(*metric.HTTPLatencyMs, 10)
	case models.ConditionHTTP4XX:
		if metric.Status4xx == nil {
			return StateUnknown, ""
		}
		value := float64(*metric.Status4xx)
		return condition(compareNumeric(value, threshold, rule.Operator)), strconv.FormatInt(*metric.Status4xx, 10)
	case models.ConditionHTTP5XX:
		if metric.Status5xx == nil {
			return StateUnknown, ""
		}
		value := float64(*metric.Status5xx)
		return condition(compareNumeric(value, threshold, rule.Operator)), strconv.FormatInt(*metric.Status5xx, 10)
	}
	return StateUnknown, ""
}

func condition(value bool) ConditionState {
	if value {
		return StateTrue
	}
	return StateFalse
}

func (e *Evaluator) EvaluateOnce(ctx context.Context) ([]RuleEvaluationResult, error) {
	rules, err := e.rules.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	snapshot, err := e.provider.Load(ctx, rules)
	if err != nil {
		return nil, err
	}
	now := e.now()
	e.mu.Lock()
	active := make(map[int64]struct{}, len(rules))
	results := make([]RuleEvaluationResult, 0, len(rules))
	evaluatedRules := make([]models.AlertRule, 0, len(rules))
	for _, rule := range rules {
		if !rule.IsEnabled {
			continue
		}
		active[rule.ID] = struct{}{}
		state, value := evaluateCondition(rule, snapshot, now, e.restartSeen)
		result := RuleEvaluationResult{RuleID: rule.ID, State: state, EvaluatedAt: now, CurrentValue: value}
		switch state {
		case StateTrue:
			pending, exists := e.pending[rule.ID]
			if !exists {
				pending = pendingState{StartedAt: now}
			}
			pending.LastEvaluatedAt = now
			e.pending[rule.ID] = pending
			started := pending.StartedAt
			result.ConditionStartedAt = &started
			result.DurationSatisfied = now.Sub(started) >= time.Duration(rule.DurationSeconds)*time.Second
			if result.DurationSatisfied && !e.results[rule.ID].DurationSatisfied {
				log.Printf("[INFO] alert rule %d condition duration satisfied", rule.ID)
			}
		case StateFalse, StateUnknown:
			delete(e.pending, rule.ID)
		}
		e.results[rule.ID] = result
		results = append(results, result)
		evaluatedRules = append(evaluatedRules, rule)
	}
	for id := range e.pending {
		if _, ok := active[id]; !ok {
			delete(e.pending, id)
		}
	}
	for id := range e.restartSeen {
		if _, ok := active[id]; !ok {
			delete(e.restartSeen, id)
		}
	}
	for id := range e.results {
		if _, ok := active[id]; !ok {
			delete(e.results, id)
		}
	}
	sink := e.sink
	e.mu.Unlock()
	if sink == nil {
		return results, nil
	}
	var persistenceErrors []error
	for i, result := range results {
		if err := sink.Persist(ctx, evaluatedRules[i], result); err != nil {
			persistenceErrors = append(persistenceErrors, err)
		}
	}
	if err := sink.Reconcile(ctx, now); err != nil {
		persistenceErrors = append(persistenceErrors, err)
	}
	return results, errors.Join(persistenceErrors...)
}

func (e *Evaluator) Start(ctx context.Context) bool {
	if !e.running.CompareAndSwap(false, true) {
		return false
	}
	go func() {
		defer e.running.Store(false)
		if _, err := e.EvaluateOnce(ctx); err != nil {
			log.Printf("[ERROR] alert evaluation failed: %v", err)
		}
		ticker := time.NewTicker(e.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := e.EvaluateOnce(ctx); err != nil {
					log.Printf("[ERROR] alert evaluation failed: %v", err)
				}
			}
		}
	}()
	return true
}

func (e *Evaluator) Results() map[int64]RuleEvaluationResult {
	e.mu.RLock()
	defer e.mu.RUnlock()
	copy := make(map[int64]RuleEvaluationResult, len(e.results))
	for id, result := range e.results {
		copy[id] = result
	}
	return copy
}
func (e *Evaluator) PendingCount() int { e.mu.RLock(); defer e.mu.RUnlock(); return len(e.pending) }
func (e *Evaluator) String() string    { return fmt.Sprintf("alert evaluator interval=%s", e.interval) }
