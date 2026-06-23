package template

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
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

type createTemplateRequest struct {
	DeviceID   string `json:"device_id"`
	Name       string `json:"name"`
	Language   string `json:"language"`
	Category   string `json:"category"`
	Components []byte `json:"components"`
}

func (h *Handler) List(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	filter := ListFilter{Search: strings.TrimSpace(c.Query("search"))}
	if raw := strings.TrimSpace(c.Query("status")); raw != "" {
		status := Status(raw)
		filter.Status = &status
	}
	if raw := strings.TrimSpace(c.Query("device_id")); raw != "" {
		channelID, err := uuid.Parse(raw)
		if err != nil {
			return response.BadRequest(c, "invalid device_id")
		}
		filter.ChannelID = &channelID
	}

	page := repository.PageParams{
		Page:     c.QueryInt("page", 1),
		PageSize: c.QueryInt("limit", 50),
	}
	result, err := h.service.List(c.Context(), claims.TeamID, claims.UserID, string(claims.Role), filter, page)
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "ok", fiber.Map{"templates": result.Items, "pagination": result.Pagination})
}

func (h *Handler) Sync(c *fiber.Ctx) error {
	return response.Fail(c, fiber.StatusNotImplemented, "template sync pending service migration")
}

func (h *Handler) Create(c *fiber.Ctx) error {
	return response.Fail(c, fiber.StatusNotImplemented, "template creation is managed in Meta Business Manager")
}

func (h *Handler) mapError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, team.ErrMemberNotFound), errors.Is(err, serviceerrors.ErrForbidden):
		return response.Forbidden(c, err.Error())
	case errors.Is(err, repository.ErrNotFound):
		return response.NotFound(c, err.Error())
	default:
		if serviceerrors.IsValidation(err) {
			return response.BadRequest(c, err.Error())
		}
		return response.InternalError(c, "internal server error")
	}
}
