package dto

import (
	"time"

	"vpsmonitoring-backend/internal/auth/models"
)

// LoginRequest defines credentials for authentication
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RegisterRequest defines inputs for public self-registration (role is strictly USER)
type RegisterRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// CreateAdminRequest defines inputs for creating an administrator
type CreateAdminRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// UpdateAdminRequest defines inputs for modifying admin profile
type UpdateAdminRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// UpdateStatusRequest defines inputs for activating/deactivating admin
type UpdateStatusRequest struct {
	Status string `json:"status"`
}

// UserResponse is the safe public representation of a user without sensitive fields
type UserResponse struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LoginResponse contains the access token and user profile
type LoginResponse struct {
	AccessToken string       `json:"access_token"`
	Token       string       `json:"token,omitempty"`
	TokenType   string       `json:"token_type"`
	User        UserResponse `json:"user"`
}

// ToUserResponse converts a User entity model to a safe UserResponse DTO
func ToUserResponse(u *models.User) UserResponse {
	if u == nil {
		return UserResponse{}
	}
	return UserResponse{
		ID:        u.ID,
		Name:      u.Name,
		Email:     u.Email,
		Role:      u.Role,
		Status:    u.Status,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}
