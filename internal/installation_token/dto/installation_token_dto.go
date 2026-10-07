package dto

import (
	"time"
)

// CreateInstallationTokenResponse is returned immediately upon token generation
// The raw token is included only in this response for inclusion in the install command.
type CreateInstallationTokenResponse struct {
	ServerID       int64     `json:"server_id"`
	Token          string    `json:"token"`
	ExpiresAt      time.Time `json:"expires_at"`
	InstallCommand string    `json:"install_command"`
}

// TokenStatusResponse provides metadata about active tokens for a server
// For security reasons, the raw token and hash are NEVER returned.
type TokenStatusResponse struct {
	Exists    bool       `json:"exists"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	IsUsed    bool       `json:"is_used"`
}

// ValidateTokenRequest represents the agent credential validation payload for Phase 2C
type ValidateTokenRequest struct {
	Token string `json:"token"`
}
