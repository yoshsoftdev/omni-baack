package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/operations"
)

type OperationsHandler struct {
	svc *operations.Service
}

func NewOperationsHandler(svc *operations.Service) *OperationsHandler {
	return &OperationsHandler{svc: svc}
}

func (h *OperationsHandler) BroadcastStatus(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	status, err := h.svc.GetBroadcastStatus(c.Context(), actor.TeamID, actor.UserID, actor.Role)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", status)
}

func (h *OperationsHandler) WorkerStatus(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	status, err := h.svc.GetWorkerStatus(c.Context(), actor.TeamID, actor.UserID, actor.Role)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", status)
}
