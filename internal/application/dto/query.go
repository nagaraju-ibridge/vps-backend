package dto

import (
	"encoding/json"
	"time"
)

// ApplicationMetricResponse is the safe user-facing representation of a single
// application_metrics row. It deliberately omits all internal IDs and relation keys.
type ApplicationMetricResponse struct {
	CollectedAt time.Time `json:"collected_at"`
	Status      string    `json:"status"` // UP | DEGRADED | DOWN | UNKNOWN

	// Process evidence
	ProcessMatched   bool       `json:"process_matched"`
	PrimaryPID       *int64     `json:"primary_pid,omitempty"`
	ProcessCount     int        `json:"process_count"`
	PrimaryStartTime *time.Time `json:"primary_start_time,omitempty"`
	CPUPercent       *float64   `json:"cpu_percent,omitempty"`
	MemoryBytes      *uint64    `json:"memory_bytes,omitempty"`

	// Port evidence
	PortConfigured bool  `json:"port_configured"`
	PortListening  *bool `json:"port_listening,omitempty"`

	// HTTP evidence
	HTTPConfigured bool    `json:"http_configured"`
	HTTPAvailable  *bool   `json:"http_available,omitempty"`
	HTTPStatusCode *int    `json:"http_status_code,omitempty"`
	HTTPLatencyMs  *int64  `json:"http_latency_ms,omitempty"`
	HTTPErrorClass *string `json:"http_error_class,omitempty"`

	// Access-log request rate evidence
	LogConfigured     bool     `json:"log_configured"`
	LogAvailable      *bool    `json:"log_available,omitempty"`
	TotalRequests     *int64   `json:"total_requests,omitempty"`
	RequestsPerSecond *float64 `json:"requests_per_second,omitempty"`
	Status2xx         *int64   `json:"status_2xx,omitempty"`
	Status3xx         *int64   `json:"status_3xx,omitempty"`
	Status4xx         *int64   `json:"status_4xx,omitempty"`
	Status5xx         *int64   `json:"status_5xx,omitempty"`
	StatusOther       *int64   `json:"status_other,omitempty"`

	// Response time evidence
	ActiveResponseCount  *int64 `json:"active_response_count,omitempty"`
	ActiveResponseAvgMs  *int64 `json:"active_response_avg_ms,omitempty"`
	PassiveResponseCount *int64 `json:"passive_response_count,omitempty"`
	PassiveResponseAvgMs *int64 `json:"passive_response_avg_ms,omitempty"`
}

// ApplicationEventResponse is the safe user-facing representation of a single
// application_events row. It deliberately omits all internal/relation IDs.
type ApplicationEventResponse struct {
	EventTime time.Time       `json:"event_time"`
	EventType string          `json:"event_type"`
	OldPID    *int64          `json:"old_pid,omitempty"`
	NewPID    *int64          `json:"new_pid,omitempty"`
	Details   json.RawMessage `json:"details,omitempty"`
}
