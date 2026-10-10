package controller

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"gofr.dev/pkg/gofr"

	agentMw "vpsmonitoring-backend/internal/agent/middleware"
	authMw "vpsmonitoring-backend/internal/auth/middleware"
	"vpsmonitoring-backend/internal/discovery/dto"
	"vpsmonitoring-backend/internal/discovery/service"
)

type DiscoveryController struct {
	discoverySvc service.DiscoveryService
}

func NewDiscoveryController(svc service.DiscoveryService) *DiscoveryController {
	return &DiscoveryController{discoverySvc: svc}
}

func (c *DiscoveryController) IngestDiscovery(ctx *gofr.Context) (any, error) {
	identity, err := agentMw.GetAgentIdentity(ctx)
	if err != nil {
		return nil, errors.New("unauthenticated request: agent identity not found")
	}

	idStr := ctx.PathParam("agentId")
	if idStr == "" || identity.AgentID != idStr {
		return nil, errors.New("unauthorized: agent ID mismatch")
	}

	var payload dto.DiscoveryPayloadDTO
	if err := ctx.Bind(&payload); err != nil {
		return nil, fmt.Errorf("invalid JSON: %v", err)
	}
	log.Printf("[DEBUG] Discovery payload received: %d website candidates, %d hosting users", len(payload.Websites), len(payload.HostingUsers))

	received, err := c.discoverySvc.IngestDiscovery(ctx.Context, identity.ServerID, payload)
	if err != nil {
		return nil, err
	}

	return dto.IngestDiscoveryResponse{
		Status:   "success",
		Received: received,
	}, nil
}

func (c *DiscoveryController) GetDiscovery(ctx *gofr.Context) (any, error) {
	claims, err := authMw.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	idStr := ctx.PathParam("serverId")
	serverID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || serverID <= 0 {
		return nil, errors.New("valid server ID is required")
	}

	return c.discoverySvc.GetDiscovery(ctx.Context, serverID, claims.UserID)
}

func (c *DiscoveryController) ProbeDomain(ctx *gofr.Context) (any, error) {
	claims, err := authMw.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	idStr := ctx.PathParam("serverId")
	serverID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || serverID <= 0 {
		return nil, errors.New("valid server ID is required")
	}

	domain := strings.TrimSpace(ctx.Param("domain"))
	if domain == "" {
		return nil, errors.New("domain parameter is required")
	}

	return c.discoverySvc.ProbeDomain(ctx.Context, serverID, claims.UserID, domain)
}

