package controller

import (
	"errors"
	"fmt"
	"strconv"

	"gofr.dev/pkg/gofr"

	agentMw "vpsmonitoring-backend/internal/agent/middleware"
	authMw "vpsmonitoring-backend/internal/auth/middleware"
	"vpsmonitoring-backend/internal/health/dto"
	"vpsmonitoring-backend/internal/health/service"
)

type HealthController struct {
	healthSvc service.HealthService
}

func NewHealthController(svc service.HealthService) *HealthController {
	return &HealthController{healthSvc: svc}
}

func (c *HealthController) CreateConfig(ctx *gofr.Context) (any, error) {
	claims, err := authMw.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	idStr := ctx.PathParam("serverId")
	serverID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || serverID <= 0 {
		return nil, errors.New("valid server ID is required")
	}

	var req dto.CreateHealthConfigRequest
	if err := ctx.Bind(&req); err != nil {
		// GoFr automatically maps returning an error to 400 if it's considered a bad request.
		// However, to ensure 400 we can just return a formatted error or let the framework handle it.
		// A standard practice in this project is returning error.
		return nil, fmt.Errorf("invalid request payload: %v", err)
	}

	// Service layer handles authorization (server ownership) and validation
	config, err := c.healthSvc.CreateConfig(ctx.Context, serverID, claims.UserID, req)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func (c *HealthController) ListConfigs(ctx *gofr.Context) (any, error) {
	claims, err := authMw.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	idStr := ctx.PathParam("serverId")
	serverID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || serverID <= 0 {
		return nil, errors.New("valid server ID is required")
	}

	configs, err := c.healthSvc.ListConfigs(ctx.Context, serverID, claims.UserID)
	if err != nil {
		return nil, err
	}

	return configs, nil
}

func (c *HealthController) DeleteConfig(ctx *gofr.Context) (any, error) {
	claims, err := authMw.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	serverIDStr := ctx.PathParam("serverId")
	serverID, err := strconv.ParseInt(serverIDStr, 10, 64)
	if err != nil || serverID <= 0 {
		return nil, errors.New("valid server ID is required")
	}

	configIDStr := ctx.PathParam("configId")
	configID, err := strconv.ParseInt(configIDStr, 10, 64)
	if err != nil || configID <= 0 {
		return nil, errors.New("valid config ID is required")
	}

	err = c.healthSvc.DeleteConfig(ctx.Context, serverID, claims.UserID, configID)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (c *HealthController) GetAgentConfig(ctx *gofr.Context) (any, error) {
	identity, err := agentMw.GetAgentIdentity(ctx)
	if err != nil {
		return nil, errors.New("unauthenticated request: agent identity not found")
	}

	// Route parameter validation
	idStr := ctx.PathParam("agentId")
	if idStr == "" || identity.AgentID != idStr {
		// Agent can only retrieve its own configuration
		return nil, errors.New("unauthorized: agent ID mismatch")
	}

	configs, err := c.healthSvc.GetAgentConfig(ctx.Context, identity.ServerID)
	if err != nil {
		return nil, err
	}

	return configs, nil
}

func (c *HealthController) IngestHealthChecks(ctx *gofr.Context) (any, error) {
	identity, err := agentMw.GetAgentIdentity(ctx)
	if err != nil {
		return nil, errors.New("unauthenticated request: agent identity not found")
	}

	idStr := ctx.PathParam("agentId")
	if idStr == "" || identity.AgentID != idStr {
		return nil, errors.New("unauthorized: agent ID mismatch")
	}

	var payload dto.AgentHealthCheckPayload
	if err := ctx.Bind(&payload); err != nil {
		return nil, fmt.Errorf("invalid request payload: %w", err)
	}

	err = c.healthSvc.IngestHealthChecks(ctx.Context, identity.ServerID, payload)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{"status": "success"}, nil
}

func (c *HealthController) GetHealthChecks(ctx *gofr.Context) (any, error) {
	claims, err := authMw.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	idStr := ctx.PathParam("serverId")
	serverID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || serverID <= 0 {
		return nil, errors.New("valid server ID is required")
	}

	results, err := c.healthSvc.GetHealthChecks(ctx.Context, serverID, claims.UserID)
	if err != nil {
		return nil, err
	}

	return results, nil
}
