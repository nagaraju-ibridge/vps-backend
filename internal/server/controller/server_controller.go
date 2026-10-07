package controller

import (
	"errors"
	"strconv"

	"gofr.dev/pkg/gofr"

	"vpsmonitoring-backend/internal/auth/middleware"
	"vpsmonitoring-backend/internal/server/dto"
	"vpsmonitoring-backend/internal/server/service"
)

// ServerController handles HTTP endpoints for server lifecycle operations
type ServerController struct {
	serverService service.ServerService
}

// NewServerController creates an instance of ServerController
func NewServerController(serverService service.ServerService) *ServerController {
	return &ServerController{serverService: serverService}
}

// ListServers handles GET /api/v1/user/servers
func (c *ServerController) ListServers(ctx *gofr.Context) (any, error) {
	claims, err := middleware.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	return c.serverService.GetUserServers(ctx, claims.UserID)
}

// CreateServer handles POST /api/v1/user/servers
func (c *ServerController) CreateServer(ctx *gofr.Context) (any, error) {
	claims, err := middleware.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	var req dto.CreateServerRequest
	if err := ctx.Bind(&req); err != nil {
		return nil, errors.New("invalid request body")
	}

	return c.serverService.CreateServer(ctx, claims.UserID, req)
}

// GetServer handles GET /api/v1/user/servers/{id}
func (c *ServerController) GetServer(ctx *gofr.Context) (any, error) {
	claims, err := middleware.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	idStr := ctx.PathParam("id")
	serverID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || serverID <= 0 {
		return nil, errors.New("valid server ID is required")
	}

	return c.serverService.GetServer(ctx, serverID, claims.UserID)
}

// UpdateServer handles PUT /api/v1/user/servers/{id}
func (c *ServerController) UpdateServer(ctx *gofr.Context) (any, error) {
	claims, err := middleware.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	idStr := ctx.PathParam("id")
	serverID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || serverID <= 0 {
		return nil, errors.New("valid server ID is required")
	}

	var req dto.UpdateServerRequest
	if err := ctx.Bind(&req); err != nil {
		return nil, errors.New("invalid request body")
	}

	return c.serverService.UpdateServer(ctx, serverID, claims.UserID, req)
}

// DeleteServer handles DELETE /api/v1/user/servers/{id}
func (c *ServerController) DeleteServer(ctx *gofr.Context) (any, error) {
	claims, err := middleware.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	idStr := ctx.PathParam("id")
	serverID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || serverID <= 0 {
		return nil, errors.New("valid server ID is required")
	}

	if err := c.serverService.DeleteServer(ctx, serverID, claims.UserID); err != nil {
		return nil, err
	}

	return map[string]any{
		"message": "server deleted successfully",
		"id":      serverID,
	}, nil
}
