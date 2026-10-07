package config_test

import (
	"strings"
	"testing"

	"vpsmonitoring-backend/internal/config"
)

func TestLoadConfig(t *testing.T) {
	t.Run("Production with missing DATABASE_URL fails startup", func(t *testing.T) {
		provider := config.MapConfigProvider{
			"APP_ENV":    "production",
			"JWT_SECRET": "prod-secret-key-12345",
		}

		cfg, err := config.LoadConfig(provider)
		if err == nil {
			t.Fatalf("expected error when DATABASE_URL is missing in production, got config: %+v", cfg)
		}
		if !strings.Contains(err.Error(), "DATABASE_URL is required in production mode") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("Production with missing JWT_SECRET fails startup", func(t *testing.T) {
		provider := config.MapConfigProvider{
			"APP_ENV":      "production",
			"DATABASE_URL": "postgresql://user:pass@ep-prod.neon.tech/db?sslmode=require",
		}

		cfg, err := config.LoadConfig(provider)
		if err == nil {
			t.Fatalf("expected error when JWT_SECRET is missing in production, got config: %+v", cfg)
		}
		if !strings.Contains(err.Error(), "JWT_SECRET is required in production mode") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("Production with both DATABASE_URL and JWT_SECRET configured succeeds", func(t *testing.T) {
		provider := config.MapConfigProvider{
			"APP_ENV":      "production",
			"DATABASE_URL": "postgresql://user:pass@ep-prod.neon.tech/db?sslmode=require",
			"JWT_SECRET":   "strong-production-jwt-secret-xyz",
		}

		cfg, err := config.LoadConfig(provider)
		if err != nil {
			t.Fatalf("unexpected error when properly configured: %v", err)
		}
		if cfg.DatabaseURL != "postgresql://user:pass@ep-prod.neon.tech/db?sslmode=require" {
			t.Errorf("unexpected DatabaseURL: %s", cfg.DatabaseURL)
		}
		if cfg.JWTSecret != "strong-production-jwt-secret-xyz" {
			t.Errorf("unexpected JWTSecret: %s", cfg.JWTSecret)
		}
		if cfg.Env != "production" {
			t.Errorf("expected env 'production', got %s", cfg.Env)
		}
	})

	t.Run("Development configuration continues to work with defaults when variables are omitted", func(t *testing.T) {
		provider := config.MapConfigProvider{
			"APP_ENV": "development",
		}

		cfg, err := config.LoadConfig(provider)
		if err != nil {
			t.Fatalf("unexpected error in development mode: %v", err)
		}
		if cfg.DatabaseURL == "" {
			t.Errorf("expected development fallback for DatabaseURL")
		}
		if cfg.JWTSecret == "" {
			t.Errorf("expected development fallback for JWTSecret")
		}
	})

	t.Run("Development configuration respects custom environment variables when provided", func(t *testing.T) {
		provider := config.MapConfigProvider{
			"APP_ENV":      "development",
			"DATABASE_URL": "postgresql://mydevuser:devpass@localhost:5432/mydevdb",
			"JWT_SECRET":   "custom-dev-secret",
		}

		cfg, err := config.LoadConfig(provider)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.DatabaseURL != "postgresql://mydevuser:devpass@localhost:5432/mydevdb" {
			t.Errorf("expected custom DatabaseURL, got %s", cfg.DatabaseURL)
		}
		if cfg.JWTSecret != "custom-dev-secret" {
			t.Errorf("expected custom JWTSecret, got %s", cfg.JWTSecret)
		}
	})
}
