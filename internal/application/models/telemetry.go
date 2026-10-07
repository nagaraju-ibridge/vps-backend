package models

import (
	"encoding/json"
	"time"
)

type ApplicationMetric struct {
	ID            int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ApplicationID int64     `gorm:"not null;index:idx_app_metrics_collected" json:"application_id"`
	CollectedAt   time.Time `gorm:"not null;index:idx_app_metrics_collected" json:"collected_at"`
	Status        string    `gorm:"type:varchar(50);not null" json:"status"` // UP, DOWN, DEGRADED, UNKNOWN

	// Process
	ProcessMatched   bool       `gorm:"not null" json:"process_matched"`
	ProcessCount     int        `gorm:"not null" json:"process_count"`
	PrimaryPID       *int64     `gorm:"type:bigint" json:"primary_pid"`
	PrimaryStartTime *time.Time `json:"primary_start_time"`
	CPUPercent       *float64   `gorm:"type:double precision" json:"cpu_percent"`
	MemoryBytes      *uint64    `gorm:"type:bigint" json:"memory_bytes"`

	// Port
	PortConfigured bool  `gorm:"not null" json:"port_configured"`
	PortListening  *bool `json:"port_listening"`

	// HTTP
	HTTPConfigured bool    `gorm:"not null" json:"http_configured"`
	HTTPAvailable  *bool   `json:"http_available"`
	HTTPStatusCode *int    `json:"http_status_code"`
	HTTPLatencyMs  *int64  `json:"http_latency_ms"`
	HTTPErrorClass *string `gorm:"type:varchar(100)" json:"http_error_class"`

	// Request Rate (Passive logs)
	LogConfigured     bool     `gorm:"not null" json:"log_configured"`
	LogAvailable      *bool    `json:"log_available"`
	TotalRequests     *int64   `json:"total_requests"`
	RequestsPerSecond *float64 `gorm:"type:double precision" json:"requests_per_second"`
	Status2xx         *int64   `json:"status_2xx"`
	Status3xx         *int64   `json:"status_3xx"`
	Status4xx         *int64   `json:"status_4xx"`
	Status5xx         *int64   `json:"status_5xx"`
	StatusOther       *int64   `json:"status_other"`

	// Response Time (Active & Passive aggregated for dashboard or persisted independently?
	// The approved architecture typically stores them in the metric row or separate.
	// We'll store active and passive separately in the row to avoid over-complicating).
	ActiveResponseCount  *int64 `json:"active_response_count"`
	ActiveResponseAvgMs  *int64 `json:"active_response_avg_ms"`
	PassiveResponseCount *int64 `json:"passive_response_count"`
	PassiveResponseAvgMs *int64 `json:"passive_response_avg_ms"`
}

func (ApplicationMetric) TableName() string {
	return "application_metrics"
}

type ApplicationEvent struct {
	ID            int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	ApplicationID int64           `gorm:"not null;index:idx_app_events_time" json:"application_id"`
	EventType     string          `gorm:"type:varchar(50);not null" json:"event_type"`
	EventTime     time.Time       `gorm:"not null;index:idx_app_events_time" json:"event_time"`
	OldPID        *int64          `gorm:"type:bigint" json:"old_pid"`
	NewPID        *int64          `gorm:"type:bigint" json:"new_pid"`
	Details       json.RawMessage `gorm:"type:jsonb" json:"details"`
}

func (ApplicationEvent) TableName() string {
	return "application_events"
}
