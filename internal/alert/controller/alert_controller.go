package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"gofr.dev/pkg/gofr"
	"vpsmonitoring-backend/internal/alert/dto"
	"vpsmonitoring-backend/internal/alert/service"
)

type AlertController struct{ service service.AlertQueryService }

func NewAlertController(service service.AlertQueryService) *AlertController {
	return &AlertController{service: service}
}

type alertStatusError struct {
	code    int
	message string
}

func (e alertStatusError) Error() string   { return e.message }
func (e alertStatusError) StatusCode() int { return e.code }
func newAlertStatusError(code int, message string) error {
	return alertStatusError{code: code, message: message}
}

func parsePositive(value, name string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, newAlertStatusError(http.StatusBadRequest, "valid "+name+" is required")
	}
	return id, nil
}
func parseAlertFilter(ctx *gofr.Context, allowResourceFilters bool) (dto.AlertListFilter, error) {
	filter := dto.AlertListFilter{Page: 1, Limit: service.DefaultAlertLimit, Status: ctx.Param("status"), Severity: ctx.Param("severity"), ConditionType: ctx.Param("condition_type")}
	if value := ctx.Param("page"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return filter, newAlertStatusError(http.StatusBadRequest, service.ErrInvalidAlertPage.Error())
		}
		filter.Page = parsed
	}
	if value := ctx.Param("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return filter, newAlertStatusError(http.StatusBadRequest, service.ErrInvalidAlertLimit.Error())
		}
		filter.Limit = parsed
	}
	if allowResourceFilters {
		if value := ctx.Param("server_id"); value != "" {
			id, err := parsePositive(value, "server_id")
			if err != nil {
				return filter, err
			}
			filter.ServerID = &id
		}
		if value := ctx.Param("application_id"); value != "" {
			id, err := parsePositive(value, "application_id")
			if err != nil {
				return filter, err
			}
			filter.ApplicationID = &id
		}
	}
	return filter, nil
}

func mapAlertQueryError(err error) error {
	switch {
	case errors.Is(err, service.ErrInvalidAlertPage), errors.Is(err, service.ErrInvalidAlertLimit), errors.Is(err, service.ErrInvalidAlertStatus), errors.Is(err, service.ErrInvalidAlertSeverity), errors.Is(err, service.ErrInvalidAlertCondition):
		return newAlertStatusError(http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrAlertNotFound), errors.Is(err, service.ErrAlertResourceNotFound):
		return newAlertStatusError(http.StatusNotFound, "resource not found")
	default:
		return newAlertStatusError(http.StatusInternalServerError, "unable to retrieve alerts")
	}
}
func alertUserID(ctx *gofr.Context) (int64, error) {
	id, err := userID(ctx)
	if err != nil {
		return 0, newAlertStatusError(http.StatusUnauthorized, "unauthenticated request")
	}
	return id, nil
}

func (c *AlertController) List(ctx *gofr.Context) (any, error) {
	uid, err := alertUserID(ctx)
	if err != nil {
		return nil, err
	}
	filter, err := parseAlertFilter(ctx, true)
	if err != nil {
		return nil, err
	}
	response, err := c.service.List(ctx.Context, uid, filter)
	if err != nil {
		return nil, mapAlertQueryError(err)
	}
	return response, nil
}
func (c *AlertController) Get(ctx *gofr.Context) (any, error) {
	uid, err := alertUserID(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(ctx.PathParam("id"))
	if err != nil {
		return nil, newAlertStatusError(http.StatusBadRequest, "valid alert ID is required")
	}
	response, err := c.service.Get(ctx.Context, uid, id)
	if err != nil {
		return nil, mapAlertQueryError(err)
	}
	return response, nil
}
func (c *AlertController) ListByServer(ctx *gofr.Context) (any, error) {
	uid, err := alertUserID(ctx)
	if err != nil {
		return nil, err
	}
	serverID, err := parsePositive(ctx.PathParam("serverId"), "server ID")
	if err != nil {
		return nil, err
	}
	filter, err := parseAlertFilter(ctx, false)
	if err != nil {
		return nil, err
	}
	response, err := c.service.ListByServer(ctx.Context, uid, serverID, filter)
	if err != nil {
		return nil, mapAlertQueryError(err)
	}
	return response, nil
}
func (c *AlertController) ListByApplication(ctx *gofr.Context) (any, error) {
	uid, err := alertUserID(ctx)
	if err != nil {
		return nil, err
	}
	serverID, err := parsePositive(ctx.PathParam("serverId"), "server ID")
	if err != nil {
		return nil, err
	}
	applicationID, err := parsePositive(ctx.PathParam("applicationId"), "application ID")
	if err != nil {
		return nil, err
	}
	filter, err := parseAlertFilter(ctx, false)
	if err != nil {
		return nil, err
	}
	response, err := c.service.ListByApplication(ctx.Context, uid, serverID, applicationID, filter)
	if err != nil {
		return nil, mapAlertQueryError(err)
	}
	return response, nil
}
