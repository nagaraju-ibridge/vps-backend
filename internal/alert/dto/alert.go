package dto

import (
	"time"

	"github.com/google/uuid"
)

type AlertResponse struct {
	ID                uuid.UUID  `json:"id"`
	RuleID            int64      `json:"rule_id"`
	ServerID          int64      `json:"server_id"`
	ApplicationID     *int64     `json:"application_id"`
	RuleName          string     `json:"rule_name"`
	Severity          string     `json:"severity"`
	ConditionType     string     `json:"condition_type"`
	Status            string     `json:"status"`
	Message           string     `json:"message"`
	FirstTriggeredAt  time.Time  `json:"first_triggered_at"`
	LastTriggeredAt   time.Time  `json:"last_triggered_at"`
	ResolvedAt        *time.Time `json:"resolved_at"`
	ResolutionReason  *string    `json:"resolution_reason"`
	ObservedValue     *float64   `json:"observed_value"`
	LastObservedState string     `json:"last_observed_state"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type AlertListResponse struct {
	Alerts     []AlertResponse `json:"alerts"`
	Page       int             `json:"page"`
	Limit      int             `json:"limit"`
	Total      int64           `json:"total"`
	TotalPages int             `json:"total_pages"`
}

type AlertListFilter struct {
	Page          int
	Limit         int
	Status        string
	Severity      string
	ConditionType string
	ServerID      *int64
	ApplicationID *int64
}
