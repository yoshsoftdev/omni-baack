package onboarding

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/device"
	"github.com/omnichannel/omni-chan-v2/internal/team"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetStatus(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	deviceID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid device id")
	}
	status, err := h.service.GetStatus(c.Context(), claims.TeamID, claims.UserID, deviceID)
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "ok", status)
}

func (h *Handler) StartSync(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	deviceID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid device id")
	}
	status, err := h.service.StartSync(c.Context(), claims.TeamID, claims.UserID, team.Role(claims.Role), deviceID)
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "onboarding sync started", status)
}

func (h *Handler) mapError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrForbidden):
		return response.Fail(c, fiber.StatusForbidden, err.Error())
	case errors.Is(err, ErrDeviceNotConnected), errors.Is(err, ErrNotCoexDevice), errors.Is(err, ErrSyncAlreadyStarted):
		return response.BadRequest(c, err.Error())
	case errors.Is(err, device.ErrNotFound), errors.Is(err, team.ErrMemberNotFound):
		return response.NotFound(c, err.Error())
	default:
		return response.InternalError(c, "something went wrong")
	}
}
