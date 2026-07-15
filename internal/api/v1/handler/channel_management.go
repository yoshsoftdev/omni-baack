package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/channelmgmt"
)

// ChannelManagementHandler exposes multi-channel capability endpoints.
type ChannelManagementHandler struct {
	svc *channelmgmt.Service
}

func NewChannelManagementHandler(svc *channelmgmt.Service) *ChannelManagementHandler {
	return &ChannelManagementHandler{svc: svc}
}

func (h *ChannelManagementHandler) ListTypes(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	types, err := h.svc.ListTypes(c.Context(), actor.TeamID, actor.UserID, actor.Role)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", types)
}

func (h *ChannelManagementHandler) Status(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	report, err := h.svc.Status(c.Context(), actor.TeamID, actor.UserID, actor.Role)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", report)
}

func (h *ChannelManagementHandler) InstagramStatus(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	status, err := h.svc.InstagramStatus(c.Context(), actor.TeamID, actor.UserID, actor.Role)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", status)
}
