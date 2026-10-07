package dto

import (
	"time"

	"vpsmonitoring-backend/internal/server/models"
)

// CreateServerRequest defines the payload for registering a new server
// Note: user_id is NOT accepted here; it is extracted strictly from the JWT
type CreateServerRequest struct {
	Name string `json:"name"`
}

// UpdateServerRequest defines the payload for updating an existing server
// Users can only rename their server; telemetry and identity fields are protected
type UpdateServerRequest struct {
	Name string `json:"name"`
}

// ServerResponse is the public representation of a server entity
type ServerResponse struct {
	ID           int64      `json:"id"`
	UserID       int64      `json:"user_id,omitempty"`
	Name         string     `json:"name"`
	Status       string     `json:"status"`
	Hostname     *string    `json:"hostname"`
	IPAddress    *string    `json:"ip_address"`
	OS           *string    `json:"os"`
	Architecture *string    `json:"architecture"`
	AgentID      *string    `json:"agent_id,omitempty"`
	LastSeen     *time.Time `json:"last_seen"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// ToServerResponse maps a database Server model into a ServerResponse DTO
func ToServerResponse(s *models.Server) ServerResponse {
	if s == nil {
		return ServerResponse{}
	}
	return ServerResponse{
		ID:           s.ID,
		UserID:       s.UserID,
		Name:         s.Name,
		Status:       s.AgentStatus,
		Hostname:     s.Hostname,
		IPAddress:    s.IPAddress,
		OS:           s.OS,
		Architecture: s.Architecture,
		AgentID:      s.AgentID,
		LastSeen:     s.LastSeen,
		CreatedAt:    s.CreatedAt,
		UpdatedAt:    s.UpdatedAt,
	}
}
