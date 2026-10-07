package evaluator

import (
	"context"
	"testing"
	"time"

	"vpsmonitoring-backend/internal/alert/models"
	appModels "vpsmonitoring-backend/internal/application/models"
	metricModels "vpsmonitoring-backend/internal/metric/models"
)

type ruleSourceStub struct{ rules []models.AlertRule }

func (s *ruleSourceStub) ListEnabled(context.Context) ([]models.AlertRule, error) {
	return s.rules, nil
}

type snapshotStub struct{ snapshot *Snapshot }

func (s *snapshotStub) Load(context.Context, []models.AlertRule) (*Snapshot, error) {
	return s.snapshot, nil
}

type resultSinkStub struct {
	persisted  []RuleEvaluationResult
	reconciled bool
}

func (s *resultSinkStub) Persist(_ context.Context, _ models.AlertRule, result RuleEvaluationResult) error {
	s.persisted = append(s.persisted, result)
	return nil
}
func (s *resultSinkStub) Reconcile(context.Context, time.Time) error { s.reconciled = true; return nil }
func i64(v int64) *int64                                             { return &v }
func b(v bool) *bool                                                 { return &v }

func serverRule(id int64, condition, operator, threshold string) models.AlertRule {
	return models.AlertRule{ID: id, UserID: 1, ServerID: 1, Name: "rule", ScopeType: models.ScopeServer, ConditionType: condition, Operator: operator, Threshold: threshold, IsEnabled: true}
}
func appRule(id int64, condition, operator, threshold string) models.AlertRule {
	appID := int64(7)
	return models.AlertRule{ID: id, UserID: 1, ServerID: 1, ApplicationID: &appID, Name: "rule", ScopeType: models.ScopeApplication, ConditionType: condition, Operator: operator, Threshold: threshold, IsEnabled: true}
}
func baseSnapshot(now time.Time) *Snapshot {
	return &Snapshot{Servers: map[int64]ServerState{1: {Exists: true, AgentStatus: "ONLINE", Metric: &metricModels.Metric{ServerID: 1, CollectedAt: now, CPUUsagePercent: 90, MemoryUsagePercent: 80, Disks: []metricModels.MetricDisk{{MountPoint: "/", UsagePercent: 70}}}}}, Applications: map[int64]ApplicationState{7: {Exists: true, Metric: &appModels.ApplicationMetric{ApplicationID: 7, CollectedAt: now, Status: "UP", HTTPConfigured: true, HTTPAvailable: b(true), HTTPLatencyMs: i64(250), Status4xx: i64(4), Status5xx: i64(1)}}}}
}

func TestNumericOperators(t *testing.T) {
	now := time.Now().UTC()
	snapshot := baseSnapshot(now)
	tests := []struct {
		name, op, threshold string
		want                ConditionState
	}{{"gt true", "GT", "80", StateTrue}, {"gt false", "GT", "90", StateFalse}, {"gte", "GTE", "90", StateTrue}, {"lt", "LT", "91", StateTrue}, {"lte", "LTE", "90", StateTrue}, {"eq", "EQ", "90", StateTrue}, {"neq", "NEQ", "80", StateTrue}}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, _ := evaluateCondition(serverRule(int64(i+1), models.ConditionServerCPU, tt.op, tt.threshold), snapshot, now, map[int64]int64{})
			if state != tt.want {
				t.Fatalf("want %s got %s", tt.want, state)
			}
		})
	}
}

func TestServerConditions(t *testing.T) {
	now := time.Now().UTC()
	snapshot := baseSnapshot(now)
	cases := []struct {
		rule models.AlertRule
		want ConditionState
	}{{serverRule(1, models.ConditionServerCPU, "GT", "89"), StateTrue}, {serverRule(2, models.ConditionServerMemory, "GTE", "80"), StateTrue}, {serverRule(3, models.ConditionServerDisk, "EQ", "70"), StateTrue}, {serverRule(4, models.ConditionServerOffline, "EQ", "OFFLINE"), StateFalse}}
	for _, tc := range cases {
		got, _ := evaluateCondition(tc.rule, snapshot, now, map[int64]int64{})
		if got != tc.want {
			t.Fatalf("%s: want %s got %s", tc.rule.ConditionType, tc.want, got)
		}
	}
	state := snapshot.Servers[1]
	state.AgentStatus = "UNKNOWN"
	snapshot.Servers[1] = state
	got, _ := evaluateCondition(serverRule(5, models.ConditionServerOffline, "EQ", "OFFLINE"), snapshot, now, map[int64]int64{})
	if got != StateUnknown {
		t.Fatalf("unknown agent status: %s", got)
	}
	state.AgentStatus = "ONLINE"
	state.Metric.CollectedAt = now.Add(-TelemetryMaxAge - time.Second)
	snapshot.Servers[1] = state
	got, _ = evaluateCondition(serverRule(6, models.ConditionServerCPU, "GT", "1"), snapshot, now, map[int64]int64{})
	if got != StateUnknown {
		t.Fatalf("stale metric: %s", got)
	}
	state.Metric.CollectedAt = now
	state.Metric.Disks = append(state.Metric.Disks, metricModels.MetricDisk{MountPoint: "/data", UsagePercent: 99})
	snapshot.Servers[1] = state
	got, _ = evaluateCondition(serverRule(7, models.ConditionServerDisk, "GT", "50"), snapshot, now, map[int64]int64{})
	if got != StateUnknown {
		t.Fatalf("multi-disk must be unknown: %s", got)
	}
}

func TestApplicationConditionsAndRestart(t *testing.T) {
	now := time.Now().UTC()
	snapshot := baseSnapshot(now)
	metric := snapshot.Applications[7].Metric
	metric.Status = "DOWN"
	metric.HTTPAvailable = b(false)
	metric.HTTPLatencyMs = i64(650)
	metric.Status4xx = i64(15)
	metric.Status5xx = i64(8)
	cases := []models.AlertRule{appRule(1, models.ConditionApplicationDown, "EQ", "DOWN"), appRule(2, models.ConditionHTTPHealthFailure, "EQ", "DOWN"), appRule(3, models.ConditionHTTPResponseTime, "GT", "500"), appRule(4, models.ConditionHTTP4XX, "GT", "10"), appRule(5, models.ConditionHTTP5XX, "GTE", "5")}
	for _, rule := range cases {
		got, _ := evaluateCondition(rule, snapshot, now, map[int64]int64{})
		if got != StateTrue {
			t.Fatalf("%s: got %s", rule.ConditionType, got)
		}
	}
	metric.Status = "DEGRADED"
	got, _ := evaluateCondition(appRule(6, models.ConditionApplicationDegrade, "EQ", "DEGRADED"), snapshot, now, map[int64]int64{})
	if got != StateTrue {
		t.Fatalf("degraded: %s", got)
	}
	metric.Status = "UNKNOWN"
	got, _ = evaluateCondition(appRule(7, models.ConditionApplicationDown, "EQ", "DOWN"), snapshot, now, map[int64]int64{})
	if got != StateUnknown {
		t.Fatalf("unknown app: %s", got)
	}
	restart := appRule(8, models.ConditionApplicationRestart, "EQ", "TRUE")
	state := snapshot.Applications[7]
	state.LatestRestartID = 10
	snapshot.Applications[7] = state
	seen := map[int64]int64{}
	got, _ = evaluateCondition(restart, snapshot, now, seen)
	if got != StateFalse {
		t.Fatalf("historical restart should establish baseline: %s", got)
	}
	state.LatestRestartID = 11
	snapshot.Applications[7] = state
	got, _ = evaluateCondition(restart, snapshot, now, seen)
	if got != StateTrue {
		t.Fatalf("new restart should be true once: %s", got)
	}
	got, _ = evaluateCondition(restart, snapshot, now, seen)
	if got != StateFalse {
		t.Fatalf("restart must not remain true: %s", got)
	}
}

func TestDurationUnknownResetAndIsolation(t *testing.T) {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	snapshot := baseSnapshot(now)
	r1 := serverRule(1, models.ConditionServerCPU, "GT", "80")
	r1.DurationSeconds = 120
	r2 := serverRule(2, models.ConditionServerMemory, "GT", "70")
	r2.DurationSeconds = 0
	source := &ruleSourceStub{rules: []models.AlertRule{r1, r2}}
	provider := &snapshotStub{snapshot: snapshot}
	e := New(source, provider, time.Second)
	e.now = func() time.Time { return now }
	results, err := e.EvaluateOnce(context.Background())
	if err != nil || results[0].DurationSatisfied || !results[1].DurationSatisfied || e.PendingCount() != 2 {
		t.Fatalf("initial results=%+v pending=%d err=%v", results, e.PendingCount(), err)
	}
	now = now.Add(2 * time.Minute)
	snapshot.Servers[1].Metric.CollectedAt = now
	results, _ = e.EvaluateOnce(context.Background())
	if !results[0].DurationSatisfied {
		t.Fatal("duration should be satisfied")
	}
	now = now.Add(time.Second)
	snapshot.Servers[1].Metric.CollectedAt = now.Add(-TelemetryMaxAge - time.Second)
	results, _ = e.EvaluateOnce(context.Background())
	if results[0].State != StateUnknown || e.PendingCount() != 0 {
		t.Fatalf("unknown must clear all pending, results=%+v pending=%d", results, e.PendingCount())
	}
	now = now.Add(time.Second)
	snapshot.Servers[1].Metric.CollectedAt = now
	results, _ = e.EvaluateOnce(context.Background())
	if results[0].DurationSatisfied {
		t.Fatal("new true period must start fresh")
	}
	source.rules = []models.AlertRule{r2}
	e.EvaluateOnce(context.Background())
	if _, ok := e.Results()[1]; ok {
		t.Fatal("deleted rule result was not cleaned")
	}
}

func TestDisabledAndDuplicateLoop(t *testing.T) {
	now := time.Now().UTC()
	disabled := serverRule(1, models.ConditionServerCPU, "GT", "1")
	disabled.IsEnabled = false
	e := New(&ruleSourceStub{rules: []models.AlertRule{disabled}}, &snapshotStub{snapshot: baseSnapshot(now)}, time.Hour)
	e.now = func() time.Time { return now }
	results, err := e.EvaluateOnce(context.Background())
	if err != nil || len(results) != 0 || e.PendingCount() != 0 {
		t.Fatalf("disabled rule evaluated: %+v %v", results, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if !e.Start(ctx) {
		t.Fatal("first start failed")
	}
	if e.Start(ctx) {
		t.Fatal("duplicate evaluator loop started")
	}
}

func TestEvaluationResultsReachPersistenceSink(t *testing.T) {
	now := time.Now().UTC()
	rule := serverRule(1, models.ConditionServerCPU, "GT", "80")
	sink := &resultSinkStub{}
	e := New(&ruleSourceStub{rules: []models.AlertRule{rule}}, &snapshotStub{snapshot: baseSnapshot(now)}, time.Hour)
	e.now = func() time.Time { return now }
	e.SetResultSink(sink)
	if _, err := e.EvaluateOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sink.persisted) != 1 || sink.persisted[0].State != StateTrue || !sink.persisted[0].DurationSatisfied || !sink.reconciled {
		t.Fatalf("sink did not receive completed cycle: %+v", sink)
	}
}
