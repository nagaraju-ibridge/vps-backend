package models

import "time"

const (
	ScopeServer      = "SERVER"
	ScopeApplication = "APPLICATION"

	ConditionServerCPU          = "SERVER_CPU"
	ConditionServerMemory       = "SERVER_MEMORY"
	ConditionServerDisk         = "SERVER_DISK"
	ConditionServerOffline      = "SERVER_OFFLINE"
	ConditionApplicationDown    = "APPLICATION_DOWN"
	ConditionApplicationDegrade = "APPLICATION_DEGRADED"
	ConditionHTTPHealthFailure  = "HTTP_HEALTH_FAILURE"
	ConditionHTTPResponseTime   = "HTTP_RESPONSE_TIME"
	ConditionHTTP4XX            = "HTTP_4XX"
	ConditionHTTP5XX            = "HTTP_5XX"
	ConditionApplicationRestart = "APPLICATION_RESTART"

	OperatorGT  = "GT"
	OperatorGTE = "GTE"
	OperatorLT  = "LT"
	OperatorLTE = "LTE"
	OperatorEQ  = "EQ"
	OperatorNEQ = "NEQ"

	SeverityInfo     = "INFO"
	SeverityWarning  = "WARNING"
	SeverityCritical = "CRITICAL"
)

type AlertRule struct {
	ID              int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID          int64     `gorm:"not null;index" json:"user_id"`
	ServerID        int64     `gorm:"not null;index" json:"server_id"`
	ApplicationID   *int64    `gorm:"index" json:"application_id"`
	Name            string    `gorm:"type:varchar(255);not null" json:"name"`
	ScopeType       string    `gorm:"type:varchar(20);not null" json:"scope_type"`
	ConditionType   string    `gorm:"type:varchar(50);not null" json:"condition_type"`
	Operator        string    `gorm:"type:varchar(10);not null" json:"operator"`
	Threshold       string    `gorm:"type:varchar(100);not null" json:"threshold"`
	DurationSeconds int       `gorm:"not null;default:0" json:"duration_seconds"`
	Severity        string    `gorm:"type:varchar(20);not null" json:"severity"`
	IsEnabled       bool      `gorm:"not null;default:true;index" json:"is_enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (AlertRule) TableName() string { return "alert_rules" }
