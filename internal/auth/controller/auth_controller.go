package controller

import (
	"errors"
	"strconv"

	"gofr.dev/pkg/gofr"

	"vpsmonitoring-backend/internal/auth/dto"
	"vpsmonitoring-backend/internal/auth/middleware"
	"vpsmonitoring-backend/internal/auth/service"
)

// AuthController handles HTTP endpoints for authentication, admin management, and user profile
type AuthController struct {
	authService service.AuthService
}

// NewAuthController creates a new instance of AuthController
func NewAuthController(authService service.AuthService) *AuthController {
	return &AuthController{authService: authService}
}

// Login handles POST /api/v1/auth/login
func (c *AuthController) Login(ctx *gofr.Context) (any, error) {
	var req dto.LoginRequest
	if err := ctx.Bind(&req); err != nil {
		return nil, errors.New("invalid request body")
	}

	return c.authService.Login(ctx, req)
}

// Register handles POST /api/v1/auth/register (strictly role = USER)
func (c *AuthController) Register(ctx *gofr.Context) (any, error) {
	var req dto.RegisterRequest
	if err := ctx.Bind(&req); err != nil {
		return nil, errors.New("invalid request body")
	}

	return c.authService.Register(ctx, req)
}

// GetAdmins handles GET /api/v1/admin/admins (requires ADMIN role)
func (c *AuthController) GetAdmins(ctx *gofr.Context) (any, error) {
	return c.authService.GetAdmins(ctx)
}

// CreateAdmin handles POST /api/v1/admin/admins (requires ADMIN role)
func (c *AuthController) CreateAdmin(ctx *gofr.Context) (any, error) {
	var req dto.CreateAdminRequest
	if err := ctx.Bind(&req); err != nil {
		return nil, errors.New("invalid request body")
	}

	return c.authService.CreateAdmin(ctx, req)
}

// UpdateAdmin handles PUT /api/v1/admin/admins/{id} (requires ADMIN role)
func (c *AuthController) UpdateAdmin(ctx *gofr.Context) (any, error) {
	idStr := ctx.PathParam("id")
	targetID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || targetID <= 0 {
		return nil, errors.New("valid admin ID is required")
	}

	var req dto.UpdateAdminRequest
	if err := ctx.Bind(&req); err != nil {
		return nil, errors.New("invalid request body")
	}

	return c.authService.UpdateAdmin(ctx, targetID, req)
}

// UpdateStatus handles PATCH /api/v1/admin/admins/{id}/status (requires ADMIN role)
func (c *AuthController) UpdateStatus(ctx *gofr.Context) (any, error) {
	idStr := ctx.PathParam("id")
	targetID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || targetID <= 0 {
		return nil, errors.New("valid admin ID is required")
	}

	var req dto.UpdateStatusRequest
	if err := ctx.Bind(&req); err != nil {
		return nil, errors.New("invalid request body")
	}

	callerClaims, err := middleware.GetClaims(ctx)
	callerID := int64(0)
	if err == nil && callerClaims != nil {
		callerID = callerClaims.UserID
	}

	return c.authService.UpdateStatus(ctx, targetID, callerID, req)
}

// GetProfile handles GET /api/v1/user/profile (requires USER or ADMIN role)
func (c *AuthController) GetProfile(ctx *gofr.Context) (any, error) {
	callerClaims, err := middleware.GetClaims(ctx)
	if err != nil {
		return nil, err
	}

	return c.authService.GetProfile(ctx, callerClaims.UserID)
}

// GetServers handles GET /api/v1/user/servers (requires USER or ADMIN role)
func (c *AuthController) GetServers(ctx *gofr.Context) (any, error) {
	callerClaims, err := middleware.GetClaims(ctx)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"owner_id": callerClaims.UserID,
		"email":    callerClaims.Email,
		"servers":  []any{},
		"message":  "VPS fleet telemetry module ready for server onboarding.",
	}, nil
}
