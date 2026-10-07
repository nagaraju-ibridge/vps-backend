package evaluator

import (
	"context"
	"time"

	"vpsmonitoring-backend/internal/alert/models"
	appModels "vpsmonitoring-backend/internal/application/models"
	metricModels "vpsmonitoring-backend/internal/metric/models"
)

type ConditionState string

const (
	StateTrue    ConditionState = "TRUE"
	StateFalse   ConditionState = "FALSE"
	StateUnknown ConditionState = "UNKNOWN"
)

type ServerState struct {
	Exists      bool
	AgentStatus string
	Metric      *metricModels.Metric
}

type ApplicationState struct {
	Exists          bool
	Metric          *appModels.ApplicationMetric
	LatestRestartID int64
}

type Snapshot struct {
	Servers      map[int64]ServerState
	Applications map[int64]ApplicationState
}

type SnapshotProvider interface {
	Load(context.Context, []models.AlertRule) (*Snapshot, error)
}

type RuleSource interface {
	ListEnabled(context.Context) ([]models.AlertRule, error)
}

type ResultSink interface {
	Persist(context.Context, models.AlertRule, RuleEvaluationResult) error
	Reconcile(context.Context, time.Time) error
}

type RuleEvaluationResult struct {
	RuleID             int64
	State              ConditionState
	ConditionStartedAt *time.Time
	DurationSatisfied  bool
	EvaluatedAt        time.Time
	CurrentValue       string
}
