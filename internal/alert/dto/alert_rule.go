package dto

import "time"

type CreateAlertRuleRequest struct {
	Name            string `json:"name"`
	ScopeType       string `json:"scope_type"`
	ServerID        int64  `json:"server_id"`
	ApplicationID   *int64 `json:"application_id"`
	ConditionType   string `json:"condition_type"`
	Operator        string `json:"operator"`
	Threshold       any    `json:"threshold"`
	DurationSeconds int    `json:"duration_seconds"`
	Severity        string `json:"severity"`
	IsEnabled       *bool  `json:"is_enabled"`
}

type UpdateAlertRuleRequest struct {
	Name            *string `json:"name"`
	ScopeType       *string `json:"scope_type"`
	ServerID        *int64  `json:"server_id"`
	ApplicationID   *int64  `json:"application_id"`
	ConditionType   *string `json:"condition_type"`
	Operator        *string `json:"operator"`
	Threshold       any     `json:"threshold"`
	DurationSeconds *int    `json:"duration_seconds"`
	Severity        *string `json:"severity"`
	IsEnabled       *bool   `json:"is_enabled"`
}

type UpdateAlertRuleStatusRequest struct {
	IsEnabled *bool `json:"is_enabled"`
}

type AlertRuleResponse struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	ScopeType       string    `json:"scope_type"`
	ServerID        int64     `json:"server_id"`
	ApplicationID   *int64    `json:"application_id,omitempty"`
	ConditionType   string    `json:"condition_type"`
	Operator        string    `json:"operator"`
	Threshold       string    `json:"threshold"`
	DurationSeconds int       `json:"duration_seconds"`
	Severity        string    `json:"severity"`
	IsEnabled       bool      `json:"is_enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
