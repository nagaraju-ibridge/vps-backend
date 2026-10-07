package models

import (
	"time"
)

// HttpHealthConfig represents a user-configured HTTP health check endpoint for a server.
type HttpHealthConfig struct {
	ID          int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ServerID    int64     `gorm:"not null;index" json:"server_id"`
	URL         string    `gorm:"type:text;not null" json:"url"`
	IntervalSec int       `gorm:"default:60;not null" json:"interval_sec"`
	IsActive    bool      `gorm:"default:true;not null" json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName explicitly defines the database table name for GORM
func (HttpHealthConfig) TableName() string {
	return "http_health_configs"
}
