package dto

import "time"

// CreateHealthConfigRequest represents the payload to create a new HTTP health check.
type CreateHealthConfigRequest struct {
	URL         string `json:"url"`
	IntervalSec *int   `json:"interval_sec,omitempty"`
	IsActive    *bool  `json:"is_active,omitempty"`
}

// HttpHealthConfigDTO represents a configured HTTP health check returned to the user.
type HttpHealthConfigDTO struct {
	ID          int64     `json:"id"`
	ServerID    int64     `json:"server_id"`
	URL         string    `json:"url"`
	IntervalSec int       `json:"interval_sec"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// HealthCheckResultDTO represents the user-facing health check result.
type HealthCheckResultDTO struct {
	ConfigID    int64     `json:"config_id"`
	URL         string    `json:"url"`
	StatusCode  int       `json:"status_code"`
	LatencyMs   int64     `json:"latency_ms"`
	IsAvailable bool      `json:"is_available"`
	ErrorClass  string    `json:"error_class"`
	CollectedAt time.Time `json:"collected_at"`
}

// AgentHealthCheckPayload represents the payload sent by the agent.
type AgentHealthCheckPayload struct {
	CollectedAt time.Time                   `json:"collected_at"`
	Checks      []AgentHealthCheckResultDTO `json:"checks"`
}

// AgentHealthCheckResultDTO is a single result inside the agent payload.
type AgentHealthCheckResultDTO struct {
	ConfigID    int64  `json:"config_id"`
	URL         string `json:"url"`
	StatusCode  int    `json:"status_code"`
	LatencyMs   int64  `json:"latency_ms"`
	IsAvailable bool   `json:"is_available"`
	ErrorClass  string `json:"error_class"`
}
