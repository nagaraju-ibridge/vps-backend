package models

import (
	"time"

	"gorm.io/gorm"
)

// Server status constants
const (
	StatusPending = "PENDING"
	StatusOnline  = "ONLINE"
	StatusOffline = "OFFLINE"
)

// Server represents a VPS instance managed by an authenticated user
type Server struct {
	ID           int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID       int64          `gorm:"not null;index" json:"user_id"`
	Name         string         `gorm:"size:255;not null" json:"name"`
	Hostname     *string        `gorm:"size:255" json:"hostname"`
	IPAddress    *string        `gorm:"size:100" json:"ip_address"`
	OS           *string        `gorm:"size:100" json:"os"`
	Architecture *string        `gorm:"size:50" json:"architecture"`
	AgentID      *string        `gorm:"size:100;index" json:"agent_id"`
	AgentStatus  string         `gorm:"size:50;not null;default:'PENDING'" json:"agent_status"`
	LastSeen     *time.Time     `json:"last_seen"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName explicitly defines the database table name for GORM
func (Server) TableName() string {
	return "servers"
}
