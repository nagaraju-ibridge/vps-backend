package controller

import (
	"errors"
	"strconv"

	"gofr.dev/pkg/gofr"

	"vpsmonitoring-backend/internal/auth/middleware"
	"vpsmonitoring-backend/internal/installation_token/service"
)

// InstallationTokenController handles HTTP endpoints for server installation tokens
type InstallationTokenController struct {
	tokenService service.InstallationTokenService
}

// NewInstallationTokenController creates a new instance of InstallationTokenController
func NewInstallationTokenController(tokenService service.InstallationTokenService) *InstallationTokenController {
	return &InstallationTokenController{tokenService: tokenService}
}

// CreateToken handles POST /api/v1/user/servers/{id}/tokens
func (c *InstallationTokenController) CreateToken(ctx *gofr.Context) (any, error) {
	claims, err := middleware.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	idStr := ctx.PathParam("id")
	serverID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || serverID <= 0 {
		return nil, errors.New("valid server ID is required")
	}

	return c.tokenService.CreateInstallationToken(ctx, serverID, claims.UserID)
}

// GetTokenStatus handles GET /api/v1/user/servers/{id}/tokens
func (c *InstallationTokenController) GetTokenStatus(ctx *gofr.Context) (any, error) {
	claims, err := middleware.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	idStr := ctx.PathParam("id")
	serverID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || serverID <= 0 {
		return nil, errors.New("valid server ID is required")
	}

	return c.tokenService.GetTokenStatus(ctx, serverID, claims.UserID)
}
