package models

import (
	"time"

	"gorm.io/gorm"
)

// Role constants
const (
	RoleAdmin = "ADMIN"
	RoleUser  = "USER"
)

// Status constants
const (
	StatusActive   = "ACTIVE"
	StatusInactive = "INACTIVE"
)

// User represents the database entity for authentication & authorization
type User struct {
	ID           int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	Name         string         `gorm:"size:255;not null" json:"name"`
	Email        string         `gorm:"size:255;not null" json:"email"`
	PasswordHash string         `gorm:"size:255;not null" json:"-"`
	Role         string         `gorm:"size:50;not null;index" json:"role"`
	Status       string         `gorm:"size:50;not null;default:'ACTIVE'" json:"status"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName overrides the table name for GORM
func (User) TableName() string {
	return "users"
}
