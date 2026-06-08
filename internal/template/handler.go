package template

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/team"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type syncRequest struct {
	DeviceID string `json:"device_id"`
}

func (h *Handler) List(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	filter := ListFilter{
		Search:   strings.TrimSpace(c.Query("search")),
		Language: strings.TrimSpace(c.Query("language")),
		Category: strings.TrimSpace(c.Query("category")),
		Status:   strings.TrimSpace(c.Query("status")),
	}
	if raw := strings.TrimSpace(c.Query("device_id")); raw != "" {
		if _, err := uuid.Parse(raw); err != nil {
			return response.BadRequest(c, "invalid device_id")
		}
		filter.DeviceID = &raw
	}

	items, err := h.service.List(c.Context(), claims.TeamID, claims.UserID, filter)
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "ok", fiber.Map{"templates": items})
}

func (h *Handler) Sync(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	var req syncRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	deviceID, err := uuid.Parse(strings.TrimSpace(req.DeviceID))
	if err != nil {
		return response.BadRequest(c, "invalid device_id")
	}

	result, err := h.service.Sync(c.Context(), claims.TeamID, claims.UserID, team.Role(claims.Role), SyncInput{
		DeviceID: deviceID,
	})
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "templates synced", result)
}

func (h *Handler) mapError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, team.ErrMemberNotFound):
		return response.Forbidden(c, err.Error())
	case errors.Is(err, ErrForbidden):
		return response.Forbidden(c, err.Error())
	case errors.Is(err, ErrDeviceNotFound):
		return response.NotFound(c, err.Error())
	case errors.Is(err, ErrDeviceNotReady), errors.Is(err, ErrDeviceIDRequired):
		return response.BadRequest(c, err.Error())
	default:
		return response.InternalError(c, "internal server error")
	}
}
