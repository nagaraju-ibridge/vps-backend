package models

import (
	"time"
)

// Agent status constants
const (
	AgentStatusActive   = "ACTIVE"
	AgentStatusRevoked  = "REVOKED"
	AgentStatusInactive = "INACTIVE"
)

// Agent represents an installed telemetry daemon connected to a specific VPS
type Agent struct {
	ID        int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	ServerID  int64      `gorm:"not null;uniqueIndex" json:"server_id"`
	AgentID   string     `gorm:"size:100;not null;uniqueIndex" json:"agent_id"`
	TokenHash string     `gorm:"size:255;not null;index" json:"-"`
	Version   string     `gorm:"size:50;not null;default:'0.1.0'" json:"version"`
	Status    string     `gorm:"size:50;not null;default:'ACTIVE'" json:"status"`
	LastSeen  *time.Time `json:"last_seen"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// TableName explicitly overrides default pluralization
func (Agent) TableName() string {
	return "agents"
}
