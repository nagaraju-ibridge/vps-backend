package controller

import (
	"errors"
	"strconv"

	"gofr.dev/pkg/gofr"
	"vpsmonitoring-backend/internal/alert/dto"
	"vpsmonitoring-backend/internal/alert/service"
	authMiddleware "vpsmonitoring-backend/internal/auth/middleware"
)

type AlertRuleController struct{ service service.AlertRuleService }

func NewAlertRuleController(service service.AlertRuleService) *AlertRuleController {
	return &AlertRuleController{service: service}
}

func userID(ctx *gofr.Context) (int64, error) {
	claims, err := authMiddleware.GetClaims(ctx)
	if err != nil || claims == nil {
		return 0, errors.New("unauthenticated request")
	}
	return claims.UserID, nil
}

func ruleID(ctx *gofr.Context) (int64, error) {
	id, err := strconv.ParseInt(ctx.PathParam("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("valid alert rule ID is required")
	}
	return id, nil
}

func (c *AlertRuleController) Create(ctx *gofr.Context) (any, error) {
	uid, err := userID(ctx)
	if err != nil {
		return nil, err
	}
	var req dto.CreateAlertRuleRequest
	if err = ctx.Bind(&req); err != nil {
		return nil, errors.New("invalid request payload")
	}
	return c.service.Create(ctx.Context, uid, req)
}
func (c *AlertRuleController) List(ctx *gofr.Context) (any, error) {
	uid, err := userID(ctx)
	if err != nil {
		return nil, err
	}
	return c.service.List(ctx.Context, uid)
}
func (c *AlertRuleController) Get(ctx *gofr.Context) (any, error) {
	uid, err := userID(ctx)
	if err != nil {
		return nil, err
	}
	id, err := ruleID(ctx)
	if err != nil {
		return nil, err
	}
	return c.service.Get(ctx.Context, uid, id)
}
func (c *AlertRuleController) Update(ctx *gofr.Context) (any, error) {
	uid, err := userID(ctx)
	if err != nil {
		return nil, err
	}
	id, err := ruleID(ctx)
	if err != nil {
		return nil, err
	}
	var req dto.UpdateAlertRuleRequest
	if err = ctx.Bind(&req); err != nil {
		return nil, errors.New("invalid request payload")
	}
	return c.service.Update(ctx.Context, uid, id, req)
}
func (c *AlertRuleController) UpdateStatus(ctx *gofr.Context) (any, error) {
	uid, err := userID(ctx)
	if err != nil {
		return nil, err
	}
	id, err := ruleID(ctx)
	if err != nil {
		return nil, err
	}
	var req dto.UpdateAlertRuleStatusRequest
	if err = ctx.Bind(&req); err != nil {
		return nil, errors.New("invalid request payload")
	}
	return c.service.UpdateStatus(ctx.Context, uid, id, req)
}
func (c *AlertRuleController) Delete(ctx *gofr.Context) (any, error) {
	uid, err := userID(ctx)
	if err != nil {
		return nil, err
	}
	id, err := ruleID(ctx)
	if err != nil {
		return nil, err
	}
	if err = c.service.Delete(ctx.Context, uid, id); err != nil {
		return nil, err
	}
	return nil, nil
}
