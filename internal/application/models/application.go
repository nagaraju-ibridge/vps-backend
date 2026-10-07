package models

import (
	"time"
)

const (
	MatchTypeSystemdUnit = "systemd_unit"
	MatchTypeExePath     = "exe_path"

	LogSourceTypeNginxAccess  = "nginx_access"
	LogSourceTypeApacheAccess = "apache_access"
)

type Application struct {
	ID             int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ServerID       int64     `gorm:"not null;index;uniqueIndex:uq_app_identity" json:"server_id"`
	Name           string    `gorm:"type:varchar(255);not null" json:"name"`
	IsEnabled      bool      `gorm:"default:true;not null" json:"is_enabled"`
	MatchType      string    `gorm:"type:varchar(50);not null;uniqueIndex:uq_app_identity" json:"match_type"`
	MatchValue     string    `gorm:"type:varchar(512);not null;uniqueIndex:uq_app_identity" json:"match_value"`
	MonitorPort    *int      `gorm:"type:int" json:"monitor_port"`
	MonitorHTTPURL *string   `gorm:"type:varchar(512)" json:"monitor_http_url"`
	LogSourceType  *string   `gorm:"type:varchar(50)" json:"log_source_type"`
	LogSourcePath  *string   `gorm:"type:varchar(512)" json:"log_source_path"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// TableName explicitly defines the database table name for GORM
func (Application) TableName() string {
	return "applications"
}
