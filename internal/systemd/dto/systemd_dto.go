package dto

import "time"

// ServiceSnapshotDTO represents a single systemd service from the agent.
type ServiceSnapshotDTO struct {
	Name        string    `json:"name" validate:"required"`
	ActiveState string    `json:"active_state" validate:"required"`
	SubState    string    `json:"sub_state" validate:"required"`
	LoadState   string    `json:"load_state" validate:"required"`
	Description string    `json:"description"`
	CollectedAt time.Time `json:"collected_at" validate:"required"`
}

// ServicePayloadDTO represents the incoming payload from the agent.
type ServicePayloadDTO struct {
	CollectedAt time.Time            `json:"collected_at" validate:"required"`
	Services    []ServiceSnapshotDTO `json:"services" validate:"required,max=200"`
}

// ServiceResponseDTO represents the response returned to the user API.
type ServiceResponseDTO struct {
	CollectedAt time.Time            `json:"collected_at"`
	Services    []ServiceSnapshotDTO `json:"services"`
}
