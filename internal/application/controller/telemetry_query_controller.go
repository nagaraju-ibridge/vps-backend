package controller

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"gofr.dev/pkg/gofr"

	"vpsmonitoring-backend/internal/application/service"
	authMw "vpsmonitoring-backend/internal/auth/middleware"
)

const (
	maxHistoryRange    = 31 * 24 * time.Hour
	maxHistoryLimit    = 44640
	maxEventsLimit     = 100
	defaultEventsLimit = 50
)

// TelemetryQueryController handles user-facing read endpoints for application telemetry.
type TelemetryQueryController struct {
	svc service.TelemetryQueryService
}

func NewTelemetryQueryController(svc service.TelemetryQueryService) *TelemetryQueryController {
	return &TelemetryQueryController{svc: svc}
}

// GetLatest handles GET /api/v1/user/servers/{serverId}/applications/{appId}/telemetry/latest
func (c *TelemetryQueryController) GetLatest(ctx *gofr.Context) (any, error) {
	claims, err := authMw.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	serverID, appID, err := parseServerAndAppID(ctx)
	if err != nil {
		return nil, err
	}

	result, err := c.svc.GetLatest(ctx.Context, claims.UserID, serverID, appID)
	if err != nil {
		return nil, mapQueryError(err)
	}
	return result, nil
}

// GetHistory handles GET /api/v1/user/servers/{serverId}/applications/{appId}/telemetry/history
func (c *TelemetryQueryController) GetHistory(ctx *gofr.Context) (any, error) {
	claims, err := authMw.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	serverID, appID, err := parseServerAndAppID(ctx)
	if err != nil {
		return nil, err
	}

	startStr := ctx.Param("start")
	if startStr == "" {
		return nil, errors.New("start timestamp is required")
	}
	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return nil, errors.New("start must be a valid RFC3339 timestamp")
	}

	endStr := ctx.Param("end")
	if endStr == "" {
		return nil, errors.New("end timestamp is required")
	}
	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		return nil, errors.New("end must be a valid RFC3339 timestamp")
	}

	// Determine limit
	limit := maxHistoryLimit
	limitStr := ctx.Param("limit")
	if limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err != nil || l <= 0 {
			return nil, errors.New("limit must be a positive integer")
		}
		if l > maxHistoryLimit {
			return nil, fmt.Errorf("limit exceeds maximum allowed value of %d", maxHistoryLimit)
		}
		limit = l
	}

	result, err := c.svc.GetHistory(ctx.Context, claims.UserID, serverID, appID, start, end, limit)
	if err != nil {
		return nil, mapQueryError(err)
	}
	return result, nil
}

// GetEvents handles GET /api/v1/user/servers/{serverId}/applications/{appId}/events
func (c *TelemetryQueryController) GetEvents(ctx *gofr.Context) (any, error) {
	claims, err := authMw.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	serverID, appID, err := parseServerAndAppID(ctx)
	if err != nil {
		return nil, err
	}

	limit := defaultEventsLimit
	limitStr := ctx.Param("limit")
	if limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err != nil || l <= 0 {
			return nil, errors.New("limit must be a positive integer")
		}
		if l > maxEventsLimit {
			return nil, fmt.Errorf("limit exceeds maximum allowed value of %d", maxEventsLimit)
		}
		limit = l
	}

	result, err := c.svc.GetEvents(ctx.Context, claims.UserID, serverID, appID, limit)
	if err != nil {
		return nil, mapQueryError(err)
	}
	return result, nil
}

// parseServerAndAppID extracts and validates serverId and appId path parameters.
func parseServerAndAppID(ctx *gofr.Context) (int64, int64, error) {
	serverID, err := strconv.ParseInt(ctx.PathParam("serverId"), 10, 64)
	if err != nil || serverID <= 0 {
		return 0, 0, errors.New("valid server ID is required")
	}

	appID, err := strconv.ParseInt(ctx.PathParam("appId"), 10, 64)
	if err != nil || appID <= 0 {
		return 0, 0, errors.New("valid application ID is required")
	}

	return serverID, appID, nil
}

// mapQueryError translates service-layer sentinel errors to user-appropriate error messages.
// We return ErrServerOwnership and ErrApplicationNotFound as generic 404-style messages
// to prevent resource enumeration.
func mapQueryError(err error) error {
	switch {
	case errors.Is(err, service.ErrServerOwnership),
		errors.Is(err, service.ErrApplicationNotFound),
		errors.Is(err, service.ErrTelemetryNotFound):
		return &notFoundError{msg: err.Error()}
	case errors.Is(err, service.ErrInvalidTimeRange),
		errors.Is(err, service.ErrTimeRangeTooLarge),
		errors.Is(err, service.ErrInvalidHistoryLimit),
		errors.Is(err, service.ErrInvalidEventsLimit):
		return err // 400-equivalent — let GoFr serialize naturally
	default:
		return err
	}
}

// notFoundError satisfies gofr's StatusCode interface so GoFr returns HTTP 404.
type notFoundError struct {
	msg string
}

func (e *notFoundError) Error() string   { return e.msg }
func (e *notFoundError) StatusCode() int { return 404 }
