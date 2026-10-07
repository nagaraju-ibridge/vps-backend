package controller

import (
	"errors"

	"gofr.dev/pkg/gofr"

	"vpsmonitoring-backend/internal/agent/dto"
	"vpsmonitoring-backend/internal/agent/middleware"
	"vpsmonitoring-backend/internal/agent/service"
)

// AgentController handles HTTP endpoints for agent operations
type AgentController struct {
	agentService service.AgentService
}

// NewAgentController initializes an AgentController instance
func NewAgentController(agentService service.AgentService) *AgentController {
	return &AgentController{agentService: agentService}
}

// Register handles POST /api/v1/agent/register
func (c *AgentController) Register(ctx *gofr.Context) (any, error) {
	var req dto.RegisterAgentRequest
	if err := ctx.Bind(&req); err != nil {
		return nil, errors.New("invalid request body")
	}

	return c.agentService.Register(ctx, req)
}

// Heartbeat handles POST /api/v1/agent/heartbeat
func (c *AgentController) Heartbeat(ctx *gofr.Context) (any, error) {
	// 1. Retrieve authenticated AgentIdentity from context (populated by AgentAuthMiddleware)
	identity, err := middleware.GetAgentIdentity(ctx)
	if err != nil || identity == nil {
		return nil, errors.New("unauthenticated request: agent identity not found")
	}

	// 2. Parse request payload
	var req dto.HeartbeatRequest
	if err := ctx.Bind(&req); err != nil {
		return nil, errors.New("invalid request body")
	}

	// 3. Delegate to Heartbeat service using authenticated AgentID and ServerID
	return c.agentService.Heartbeat(ctx, identity.AgentID, identity.ServerID, req)
}
