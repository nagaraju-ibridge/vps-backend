package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"vpsmonitoring-backend/internal/alert/models"
)

var ErrAlertRuleInactive = errors.New("alert rule is disabled or deleted")

type AlertRepository interface {
	AutoMigrate() error
	CreateOrUpdateActive(context.Context, *models.Alert) (*models.Alert, bool, error)
	ResolveActive(context.Context, int64, int64, *int64, time.Time, string, string) (bool, error)
	ResolveInactiveRules(context.Context, time.Time) error
	GetByIDAndUserID(context.Context, uuid.UUID, int64) (*models.Alert, error)
	List(context.Context, AlertQuery) ([]models.Alert, int64, error)
}

type AlertQuery struct {
	UserID        int64
	ServerID      *int64
	ApplicationID *int64
	Status        string
	Severity      string
	ConditionType string
	Limit         int
	Offset        int
}

type alertRepository struct{ db *gorm.DB }

func NewAlertRepository(db *gorm.DB) AlertRepository { return &alertRepository{db: db} }

func (r *alertRepository) AutoMigrate() error {
	if err := r.db.AutoMigrate(&models.Alert{}); err != nil {
		return err
	}
	statements := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_alert_active_server ON alerts (rule_id, server_id) WHERE status = 'ACTIVE' AND application_id IS NULL`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_alert_active_application ON alerts (rule_id, server_id, application_id) WHERE status = 'ACTIVE' AND application_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_user_created ON alerts (user_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_server_created ON alerts (server_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_application_created ON alerts (application_id, created_at DESC) WHERE application_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_status_created ON alerts (status, created_at DESC)`,
	}
	for _, statement := range statements {
		if err := r.db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func applyAlertQuery(db *gorm.DB, query AlertQuery) *gorm.DB {
	db = db.Where("user_id = ?", query.UserID)
	if query.ServerID != nil {
		db = db.Where("server_id = ?", *query.ServerID)
	}
	if query.ApplicationID != nil {
		db = db.Where("application_id = ?", *query.ApplicationID)
	}
	if query.Status != "" {
		db = db.Where("status = ?", query.Status)
	}
	if query.Severity != "" {
		db = db.Where("severity = ?", query.Severity)
	}
	if query.ConditionType != "" {
		db = db.Where("condition_type = ?", query.ConditionType)
	}
	return db
}

func (r *alertRepository) GetByIDAndUserID(ctx context.Context, id uuid.UUID, userID int64) (*models.Alert, error) {
	var alert models.Alert
	if err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&alert).Error; err != nil {
		return nil, err
	}
	return &alert, nil
}
func (r *alertRepository) List(ctx context.Context, query AlertQuery) ([]models.Alert, int64, error) {
	var total int64
	base := applyAlertQuery(r.db.WithContext(ctx).Model(&models.Alert{}), query)
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	alerts := make([]models.Alert, 0)
	if err := applyAlertQuery(r.db.WithContext(ctx).Model(&models.Alert{}), query).Order("created_at DESC, id DESC").Limit(query.Limit).Offset(query.Offset).Find(&alerts).Error; err != nil {
		return nil, 0, err
	}
	return alerts, total, nil
}

func activeQuery(db *gorm.DB, ruleID, serverID int64, applicationID *int64) *gorm.DB {
	query := db.Where("rule_id = ? AND server_id = ? AND status = ?", ruleID, serverID, models.AlertStatusActive)
	if applicationID == nil {
		return query.Where("application_id IS NULL")
	}
	return query.Where("application_id = ?", *applicationID)
}

func (r *alertRepository) CreateOrUpdateActive(ctx context.Context, alert *models.Alert) (*models.Alert, bool, error) {
	var persisted *models.Alert
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rule models.AlertRule
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND is_enabled = ?", alert.RuleID, true).First(&rule).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAlertRuleInactive
			}
			return err
		}
		if alert.ID == uuid.Nil {
			alert.ID = uuid.New()
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(alert)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			copy := *alert
			persisted = &copy
			created = true
			return nil
		}
		var existing models.Alert
		if err := activeQuery(tx, alert.RuleID, alert.ServerID, alert.ApplicationID).First(&existing).Error; err != nil {
			return err
		}
		updates := map[string]any{"last_triggered_at": alert.LastTriggeredAt, "last_observed_value": alert.LastObservedValue, "last_observed_state": alert.LastObservedState, "message": alert.Message, "severity": alert.Severity, "updated_at": alert.UpdatedAt}
		if err := tx.Model(&existing).Updates(updates).Error; err != nil {
			return err
		}
		existing.LastTriggeredAt = alert.LastTriggeredAt
		existing.LastObservedValue = alert.LastObservedValue
		existing.LastObservedState = alert.LastObservedState
		existing.Message = alert.Message
		existing.Severity = alert.Severity
		existing.UpdatedAt = alert.UpdatedAt
		persisted = &existing
		return nil
	})
	return persisted, created, err
}

func (r *alertRepository) ResolveActive(ctx context.Context, ruleID, serverID int64, applicationID *int64, at time.Time, observedState, reason string) (bool, error) {
	updates := map[string]any{"status": models.AlertStatusResolved, "resolved_at": at, "last_observed_state": observedState, "resolution_reason": reason, "updated_at": at}
	result := activeQuery(r.db.WithContext(ctx).Model(&models.Alert{}), ruleID, serverID, applicationID).Updates(updates)
	return result.RowsAffected > 0, result.Error
}

func (r *alertRepository) ResolveInactiveRules(ctx context.Context, at time.Time) error {
	result := r.db.WithContext(ctx).Model(&models.Alert{}).Where("status = ? AND NOT EXISTS (SELECT 1 FROM alert_rules WHERE alert_rules.id = alerts.rule_id AND alert_rules.is_enabled = ?)", models.AlertStatusActive, true).Updates(map[string]any{"status": models.AlertStatusResolved, "resolved_at": at, "last_observed_state": "FALSE", "resolution_reason": gorm.Expr("CASE WHEN EXISTS (SELECT 1 FROM alert_rules WHERE alert_rules.id = alerts.rule_id) THEN ? ELSE ? END", models.ResolutionRuleDisabled, models.ResolutionRuleDeleted), "updated_at": at})
	return result.Error
}
