package config

import (
	"errors"
	"strings"
)

// AppConfig holds the core application runtime secrets and connection settings.
type AppConfig struct {
	Env         string
	DatabaseURL string
	JWTSecret   string
}

// ConfigProvider abstracts reading configuration values (compatible with gofr.Config or os.Getenv).
type ConfigProvider interface {
	Get(key string) string
	GetOrDefault(key, defaultValue string) string
}

// MapConfigProvider is a lightweight ConfigProvider implementation for tests or maps.
type MapConfigProvider map[string]string

func (m MapConfigProvider) Get(key string) string {
	return m[key]
}

func (m MapConfigProvider) GetOrDefault(key, defaultValue string) string {
	if val, ok := m[key]; ok && strings.TrimSpace(val) != "" {
		return val
	}
	return defaultValue
}

// LoadConfig reads and validates application configuration from the provider.
// When running in production (APP_ENV=production or ENVIRONMENT=production),
// hardcoded fallbacks are strictly disabled and missing secrets will cause validation failure.
func LoadConfig(cfg ConfigProvider) (*AppConfig, error) {
	if cfg == nil {
		return nil, errors.New("config provider cannot be nil")
	}

	env := strings.ToLower(strings.TrimSpace(cfg.GetOrDefault("APP_ENV", cfg.GetOrDefault("ENVIRONMENT", "development"))))
	isProduction := env == "production" || env == "prod"

	dbURL := strings.TrimSpace(cfg.Get("DATABASE_URL"))
	jwtSecret := strings.TrimSpace(cfg.Get("JWT_SECRET"))

	if isProduction {
		if dbURL == "" {
			return nil, errors.New("DATABASE_URL is required in production mode; hardcoded fallbacks are strictly prohibited")
		}
		if jwtSecret == "" {
			return nil, errors.New("JWT_SECRET is required in production mode; hardcoded fallbacks are strictly prohibited")
		}
	} else {
		// In development or local testing, fallback to local dev defaults if not set in configs/.env
		if dbURL == "" {
			dbURL = cfg.GetOrDefault("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/vpsmonitoring_dev?sslmode=disable")
		}
		if jwtSecret == "" {
			jwtSecret = cfg.GetOrDefault("JWT_SECRET", "dev-only-local-secret-key-not-for-production")
		}
	}

	return &AppConfig{
		Env:         env,
		DatabaseURL: dbURL,
		JWTSecret:   jwtSecret,
	}, nil
}
