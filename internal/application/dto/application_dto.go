package dto

import "time"

type ApplicationCreateRequest struct {
	Name           string `json:"name"`
	MatchType      string `json:"match_type"`
	MatchValue     string `json:"match_value"`
	MonitorPort    int    `json:"monitor_port,omitempty"`
	MonitorHTTPURL string `json:"monitor_http_url,omitempty"`
	LogSourceType  string `json:"log_source_type,omitempty"`
	LogSourcePath  string `json:"log_source_path,omitempty"`
}

type ApplicationUpdateRequest struct {
	Name           *string `json:"name,omitempty"`
	IsEnabled      *bool   `json:"is_enabled,omitempty"`
	MatchType      *string `json:"match_type,omitempty"`
	MatchValue     *string `json:"match_value,omitempty"`
	MonitorPort    *int    `json:"monitor_port,omitempty"`
	MonitorHTTPURL *string `json:"monitor_http_url,omitempty"`
	LogSourceType  *string `json:"log_source_type,omitempty"`
	LogSourcePath  *string `json:"log_source_path,omitempty"`
}

type ApplicationResponse struct {
	ID             int64     `json:"id"`
	ServerID       int64     `json:"server_id"`
	Name           string    `json:"name"`
	IsEnabled      bool      `json:"is_enabled"`
	MatchType      string    `json:"match_type"`
	MatchValue     string    `json:"match_value"`
	MonitorPort    *int      `json:"monitor_port,omitempty"`
	MonitorHTTPURL *string   `json:"monitor_http_url,omitempty"`
	LogSourceType  *string   `json:"log_source_type,omitempty"`
	LogSourcePath  *string   `json:"log_source_path,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
