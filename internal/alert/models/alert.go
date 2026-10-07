package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	AlertStatusActive   = "ACTIVE"
	AlertStatusResolved = "RESOLVED"

	ResolutionConditionCleared = "condition_cleared"
	ResolutionRuleDisabled     = "rule_disabled"
	ResolutionRuleDeleted      = "rule_deleted"
)

type Alert struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	RuleID            int64      `gorm:"not null;index" json:"rule_id"`
	UserID            int64      `gorm:"not null;index" json:"user_id"`
	ServerID          int64      `gorm:"not null;index" json:"server_id"`
	ApplicationID     *int64     `gorm:"index" json:"application_id"`
	RuleName          string     `gorm:"type:varchar(255);not null" json:"rule_name"`
	ConditionType     string     `gorm:"type:varchar(50);not null" json:"condition_type"`
	Severity          string     `gorm:"type:varchar(20);not null" json:"severity"`
	Status            string     `gorm:"type:varchar(20);not null;index" json:"status"`
	Message           string     `gorm:"type:varchar(512);not null" json:"message"`
	FirstTriggeredAt  time.Time  `gorm:"not null" json:"first_triggered_at"`
	LastTriggeredAt   time.Time  `gorm:"not null" json:"last_triggered_at"`
	ResolvedAt        *time.Time `gorm:"index" json:"resolved_at"`
	ResolutionReason  *string    `gorm:"type:varchar(50)" json:"resolution_reason"`
	LastObservedValue *float64   `gorm:"type:double precision" json:"last_observed_value"`
	LastObservedState string     `gorm:"type:varchar(20);not null" json:"last_observed_state"`
	CreatedAt         time.Time  `gorm:"not null;index" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"not null" json:"updated_at"`
}

func (Alert) TableName() string { return "alerts" }
