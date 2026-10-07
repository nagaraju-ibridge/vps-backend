package models

import (
	"time"
)

// InstallationToken represents a temporary, one-time authentication credential
// used by the agent during installation to verify association with a specific server.
// Relational hierarchy: User -> Server -> Installation Token -> Agent
type InstallationToken struct {
	ID        int64      `gorm:"primaryKey;autoIncrement" json:"id"`
	ServerID  int64      `gorm:"not null;index" json:"server_id"`
	TokenHash string     `gorm:"size:255;not null;index" json:"-"`
	ExpiresAt time.Time  `gorm:"not null;index" json:"expires_at"`
	IsUsed    bool       `gorm:"not null;default:false" json:"is_used"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

// TableName explicitly overrides GORM default pluralization
func (InstallationToken) TableName() string {
	return "installation_tokens"
}

// IsExpired returns true if the token lifetime has elapsed
func (t *InstallationToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// IsValid checks if the token is eligible for agent verification (not used and not expired)
func (t *InstallationToken) IsValid() bool {
	return !t.IsUsed && !t.IsExpired()
}
