package controller

import (
	"errors"
	"gofr.dev/pkg/gofr"
	agentMw "vpsmonitoring-backend/internal/agent/middleware"
	"vpsmonitoring-backend/internal/application/dto"
	"vpsmonitoring-backend/internal/application/service"
)

type TelemetryController interface {
	IngestTelemetry(ctx *gofr.Context) (any, error)
}

type telemetryController struct {
	telemetryService service.TelemetryService
}

func NewTelemetryController(telemetryService service.TelemetryService) TelemetryController {
	return &telemetryController{
		telemetryService: telemetryService,
	}
}

func (ctrl *telemetryController) IngestTelemetry(ctx *gofr.Context) (any, error) {
	identity, err := agentMw.GetAgentIdentity(ctx)
	if err != nil {
		return nil, errors.New("unauthorized server identity")
	}

	idStr := ctx.PathParam("agentId")
	if idStr == "" || identity.AgentID != idStr {
		return nil, errors.New("unauthorized server identity")
	}

	serverID := identity.ServerID

	var batch dto.ApplicationTelemetryBatch
	if err := ctx.Bind(&batch); err != nil {
		return nil, errors.New("invalid payload: " + err.Error())
	}

	if err := ctrl.telemetryService.IngestTelemetry(ctx.Context, serverID, batch); err != nil {
		return nil, errors.New(err.Error())
	}

	return map[string]string{"status": "accepted"}, nil
}
