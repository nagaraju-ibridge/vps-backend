package controller

import (
	"errors"
	"strconv"

	"gofr.dev/pkg/gofr"

	agentMw "vpsmonitoring-backend/internal/agent/middleware"
	"vpsmonitoring-backend/internal/application/dto"
	"vpsmonitoring-backend/internal/application/service"
	authMw "vpsmonitoring-backend/internal/auth/middleware"
)

type ApplicationController struct {
	appSvc service.ApplicationService
}

func NewApplicationController(svc service.ApplicationService) *ApplicationController {
	return &ApplicationController{appSvc: svc}
}

func (c *ApplicationController) Create(ctx *gofr.Context) (any, error) {
	claims, err := authMw.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	idStr := ctx.PathParam("serverId")
	serverID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || serverID <= 0 {
		return nil, errors.New("valid server ID is required")
	}

	var req dto.ApplicationCreateRequest
	if err := ctx.Bind(&req); err != nil {
		return nil, errors.New("invalid request payload")
	}

	resp, err := c.appSvc.Create(ctx.Context, claims.UserID, serverID, req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *ApplicationController) List(ctx *gofr.Context) (any, error) {
	claims, err := authMw.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	idStr := ctx.PathParam("serverId")
	serverID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || serverID <= 0 {
		return nil, errors.New("valid server ID is required")
	}

	apps, err := c.appSvc.List(ctx.Context, claims.UserID, serverID)
	if err != nil {
		return nil, err
	}

	// GoFr expects either slice or object, slice of DTO is perfectly fine
	return apps, nil
}

func (c *ApplicationController) Get(ctx *gofr.Context) (any, error) {
	claims, err := authMw.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	serverID, err := strconv.ParseInt(ctx.PathParam("serverId"), 10, 64)
	if err != nil || serverID <= 0 {
		return nil, errors.New("valid server ID is required")
	}

	appID, err := strconv.ParseInt(ctx.PathParam("appId"), 10, 64)
	if err != nil || appID <= 0 {
		return nil, errors.New("valid app ID is required")
	}

	resp, err := c.appSvc.Get(ctx.Context, claims.UserID, serverID, appID)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *ApplicationController) Update(ctx *gofr.Context) (any, error) {
	claims, err := authMw.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	serverID, err := strconv.ParseInt(ctx.PathParam("serverId"), 10, 64)
	if err != nil || serverID <= 0 {
		return nil, errors.New("valid server ID is required")
	}

	appID, err := strconv.ParseInt(ctx.PathParam("appId"), 10, 64)
	if err != nil || appID <= 0 {
		return nil, errors.New("valid app ID is required")
	}

	var req dto.ApplicationUpdateRequest
	if err := ctx.Bind(&req); err != nil {
		return nil, errors.New("invalid request payload")
	}

	resp, err := c.appSvc.Update(ctx.Context, claims.UserID, serverID, appID, req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *ApplicationController) Delete(ctx *gofr.Context) (any, error) {
	claims, err := authMw.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	serverID, err := strconv.ParseInt(ctx.PathParam("serverId"), 10, 64)
	if err != nil || serverID <= 0 {
		return nil, errors.New("valid server ID is required")
	}

	appID, err := strconv.ParseInt(ctx.PathParam("appId"), 10, 64)
	if err != nil || appID <= 0 {
		return nil, errors.New("valid app ID is required")
	}

	err = c.appSvc.Delete(ctx.Context, claims.UserID, serverID, appID)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func (c *ApplicationController) GetAgentConfig(ctx *gofr.Context) (any, error) {
	identity, err := agentMw.GetAgentIdentity(ctx)
	if err != nil {
		return nil, errors.New("unauthenticated request: agent identity not found")
	}

	idStr := ctx.PathParam("agentId")
	if idStr == "" || identity.AgentID != idStr {
		return nil, errors.New("unauthorized: agent ID mismatch")
	}

	configs, err := c.appSvc.GetAgentConfig(ctx.Context, identity.ServerID)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{"data": configs}, nil
}
