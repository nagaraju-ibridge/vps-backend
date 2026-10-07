package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"gofr.dev/pkg/gofr"

	agentMiddleware "vpsmonitoring-backend/internal/agent/middleware"
	authMiddleware "vpsmonitoring-backend/internal/auth/middleware"
	"vpsmonitoring-backend/internal/metric/dto"
	"vpsmonitoring-backend/internal/metric/service"
	serverService "vpsmonitoring-backend/internal/server/service"
)

const (
	// Historical metrics are raw snapshots collected every 60 seconds by default.
	// Keep requests bounded until aggregation/downsampling and retention policies exist.
	MaxHistoricalRange = 7 * 24 * time.Hour
	MaxHistoricalLimit = 10080
)

// MetricController handles HTTP endpoints for agent metric ingestion.
type MetricController struct {
	metricService service.MetricService
}

// NewMetricController instantiates a new MetricController.
func NewMetricController(metricService service.MetricService) *MetricController {
	return &MetricController{metricService: metricService}
}

// Ingest handles POST /api/v1/agent/metrics
func (c *MetricController) Ingest(ctx *gofr.Context) (any, error) {
	// 1. Retrieve authenticated AgentIdentity from context (populated by AgentAuthMiddleware)
	identity, err := agentMiddleware.GetAgentIdentity(ctx)
	if err != nil || identity == nil {
		return nil, errors.New("unauthenticated request: agent identity not found")
	}

	// 2. Parse request payload
	var req dto.IngestMetricRequest
	if err := ctx.Bind(&req); err != nil {
		return nil, errors.New("invalid request body")
	}

	// 3. Delegate to MetricService using authenticated agent identity (tenant isolation guaranteed)
	resp, err := c.metricService.IngestMetric(ctx, identity.AgentID, identity.ServerID, req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// GetLatest handles GET /api/v1/user/servers/{serverId}/metrics/latest
func (c *MetricController) GetLatest(ctx *gofr.Context) (any, error) {
	claims, err := authMiddleware.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	idStr := ctx.PathParam("serverId")
	serverID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || serverID <= 0 {
		return nil, errors.New("valid server ID is required")
	}

	return c.metricService.GetLatestMetric(ctx, serverID, claims.UserID)
}

// GetHistory handles GET /api/v1/user/servers/{serverId}/metrics/history
func (c *MetricController) GetHistory(ctx *gofr.Context) (any, error) {
	claims, err := authMiddleware.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	idStr := ctx.PathParam("serverId")
	serverID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || serverID <= 0 {
		return nil, errors.New("valid server ID is required")
	}

	startStr := ctx.Param("start")
	if startStr == "" {
		return nil, errors.New("start timestamp is required")
	}
	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return nil, errors.New("valid start timestamp is required")
	}

	endStr := ctx.Param("end")
	if endStr == "" {
		return nil, errors.New("end timestamp is required")
	}
	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		return nil, errors.New("valid end timestamp is required")
	}
	if start.After(end) {
		return nil, errors.New("start timestamp must be before or equal to end timestamp")
	}
	if end.Sub(start) > MaxHistoricalRange {
		return nil, fmt.Errorf("historical range exceeds maximum allowed duration of %s", MaxHistoricalRange)
	}

	limitStr := ctx.Param("limit")
	if limitStr == "" {
		return nil, errors.New("limit is required")
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		return nil, errors.New("valid limit is required")
	}
	if limit > MaxHistoricalLimit {
		return nil, fmt.Errorf("limit exceeds maximum allowed value of %d", MaxHistoricalLimit)
	}

	return c.metricService.GetHistoricalMetrics(ctx, serverID, claims.UserID, start, end, limit)
}

// GetAggregate handles GET /api/v1/user/servers/{serverId}/metrics/aggregate
func (c *MetricController) GetAggregate(ctx *gofr.Context) (any, error) {
	claims, err := authMiddleware.GetClaims(ctx)
	if err != nil || claims == nil {
		return nil, errors.New("unauthenticated request")
	}

	idStr := ctx.PathParam("serverId")
	serverID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || serverID <= 0 {
		return nil, newStatusError(http.StatusBadRequest, "valid server ID is required")
	}

	startStr := ctx.Param("start")
	if startStr == "" {
		return nil, newStatusError(http.StatusBadRequest, "start timestamp is required")
	}
	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return nil, newStatusError(http.StatusBadRequest, "valid start timestamp is required")
	}

	endStr := ctx.Param("end")
	if endStr == "" {
		return nil, newStatusError(http.StatusBadRequest, "end timestamp is required")
	}
	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		return nil, newStatusError(http.StatusBadRequest, "valid end timestamp is required")
	}

	granularity := ctx.Param("granularity")
	if granularity == "" {
		return nil, newStatusError(http.StatusBadRequest, "granularity is required")
	}

	resp, err := c.metricService.GetAggregatedMetrics(ctx, serverID, claims.UserID, start, end, granularity)
	if err != nil {
		return nil, mapAggregateError(err)
	}

	return resp, nil
}

type statusError struct {
	code    int
	message string
	cause   error
}

func newStatusError(code int, message string) statusError {
	return statusError{code: code, message: message}
}

func (e statusError) Error() string {
	return e.message
}

func (e statusError) StatusCode() int {
	return e.code
}

func (e statusError) Unwrap() error {
	return e.cause
}

func newStatusErrorFromErr(code int, err error) statusError {
	return statusError{code: code, message: err.Error(), cause: err}
}

func mapAggregateError(err error) error {
	switch {
	case errors.Is(err, service.ErrInvalidTimeRange),
		errors.Is(err, service.ErrInvalidGranularity),
		errors.Is(err, service.ErrBucketLimitExceeded):
		return newStatusErrorFromErr(http.StatusBadRequest, err)
	case errors.Is(err, serverService.ErrServerNotFound):
		return newStatusErrorFromErr(http.StatusNotFound, err)
	default:
		return err
	}
}
