package repository

import (
	"fmt"
	"log"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"vpsmonitoring-backend/internal/auth/models"
)

// InitGORM initializes GORM with Neon PostgreSQL and runs AutoMigrate for auth models
func InitGORM(dsn string) (*gorm.DB, error) {
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	}

	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  dsn,
		PreferSimpleProtocol: true, // Ideal for PgBouncer / Neon poolers
	}), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Neon PostgreSQL via GORM: %w", err)
	}

	// Configure underlying connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve generic database object from GORM: %w", err)
	}

	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetMaxOpenConns(30)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	// Ensure constraint compatibility for GORM postgres migrator
	_ = db.Exec(`
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'uni_users_email') THEN
				IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_email_key') THEN
					ALTER TABLE users RENAME CONSTRAINT users_email_key TO uni_users_email;
				ELSE
					ALTER TABLE users ADD CONSTRAINT uni_users_email UNIQUE (email);
				END IF;
			END IF;
		END $$;
	`).Error

	// Run GORM AutoMigrate schema for auth models
	if err := db.AutoMigrate(&models.User{}); err != nil {
		return nil, fmt.Errorf("GORM AutoMigrate failed for models: %w", err)
	}

	log.Println("[GORM] Database connection established and schema migrated successfully.")
	return db, nil
}
