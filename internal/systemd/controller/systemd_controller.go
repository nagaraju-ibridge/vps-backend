package controller

import (
	"errors"
	"fmt"
	"strconv"

	"gofr.dev/pkg/gofr"

	agentMw "vpsmonitoring-backend/internal/agent/middleware"
	authMw "vpsmonitoring-backend/internal/auth/middleware"
	"vpsmonitoring-backend/internal/systemd/dto"
	"vpsmonitoring-backend/internal/systemd/service"
)

type SystemdController struct {
	systemdSvc service.SystemdService
}

func NewSystemdController(svc service.SystemdService) *SystemdController {
	return &SystemdController{systemdSvc: svc}
}

type IngestResponse struct {
	Received int `json:"received"`
}

func (c *SystemdController) IngestSnapshot(ctx *gofr.Context) (any, error) {
	identity, err := agentMw.GetAgentIdentity(ctx)
	if err != nil {
		return nil, errors.New("unauthenticated request: agent identity not found")
	}

	var req dto.ServicePayloadDTO
	if err := ctx.Bind(&req); err != nil {
		return nil, fmt.Errorf("invalid JSON: %v", err)
	}

	if req.CollectedAt.IsZero() {
		return nil, errors.New("collected_at is required and must be a valid RFC-3339 timestamp")
	}

	if len(req.Services) > 200 {
		return nil, errors.New("exceeded maximum number of services")
	}

	err = c.systemdSvc.IngestSnapshot(identity.ServerID, req)
	if err != nil {
		return nil, err
	}

	return IngestResponse{Received: len(req.Services)}, nil
}

func (c *SystemdController) GetServices(ctx *gofr.Context) (any, error) {
	claims, err := authMw.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	idStr := ctx.PathParam("serverId")
	serverID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || serverID <= 0 {
		return nil, errors.New("valid server ID is required")
	}

	limit := 50
	limitStr := ctx.Param("limit")
	if limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil {
			return nil, service.ErrInvalidLimit
		}
		limit = parsedLimit
	}

	sortStr := ctx.Param("sort")

	resp, err := c.systemdSvc.GetLatestSnapshot(ctx.Context, serverID, claims.UserID, limit, sortStr)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
