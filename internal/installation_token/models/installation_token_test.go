package models_test

import (
	"testing"
	"time"

	"vpsmonitoring-backend/internal/installation_token/models"
)

func TestInstallationToken_Validation(t *testing.T) {
	now := time.Now()

	// 1. Valid Active Token
	activeToken := models.InstallationToken{
		ID:        1,
		ServerID:  10,
		TokenHash: "dummyhash123",
		ExpiresAt: now.Add(24 * time.Hour),
		IsUsed:    false,
	}

	if activeToken.IsExpired() {
		t.Errorf("expected token not to be expired")
	}
	if !activeToken.IsValid() {
		t.Errorf("expected token to be valid")
	}

	// 2. Expired Token
	expiredToken := models.InstallationToken{
		ID:        2,
		ServerID:  10,
		TokenHash: "dummyhash456",
		ExpiresAt: now.Add(-1 * time.Hour),
		IsUsed:    false,
	}

	if !expiredToken.IsExpired() {
		t.Errorf("expected token to be expired")
	}
	if expiredToken.IsValid() {
		t.Errorf("expected expired token to be invalid")
	}

	// 3. Used Token (not expired)
	usedToken := models.InstallationToken{
		ID:        3,
		ServerID:  10,
		TokenHash: "dummyhash789",
		ExpiresAt: now.Add(1 * time.Hour),
		IsUsed:    true,
	}

	if usedToken.IsExpired() {
		t.Errorf("expected token not to be expired yet")
	}
	if usedToken.IsValid() {
		t.Errorf("expected used token to be invalid")
	}
}
