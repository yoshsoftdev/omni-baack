package template

import (
	"encoding/json"
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

type createTemplateRequest struct {
	DeviceID   string          `json:"device_id"`
	Name       string          `json:"name"`
	Language   string          `json:"language"`
	Category   string          `json:"category"`
	Components json.RawMessage `json:"components"`
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

func (h *Handler) Create(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	var req createTemplateRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	deviceID, err := uuid.Parse(strings.TrimSpace(req.DeviceID))
	if err != nil {
		return response.BadRequest(c, "invalid device_id")
	}

	item, err := h.service.Create(c.Context(), claims.TeamID, claims.UserID, team.Role(claims.Role), CreateInput{
		DeviceID:   deviceID,
		Name:       strings.TrimSpace(req.Name),
		Language:   strings.TrimSpace(req.Language),
		Category:   strings.TrimSpace(req.Category),
		Components: req.Components,
	})
	if err != nil {
		return h.mapError(c, err)
	}
	return response.Created(c, "template submitted to meta", item)
}

func (h *Handler) mapError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, team.ErrMemberNotFound):
		return response.Forbidden(c, err.Error())
	case errors.Is(err, ErrForbidden):
		return response.Forbidden(c, err.Error())
	case errors.Is(err, ErrDeviceNotFound):
		return response.NotFound(c, err.Error())
	case errors.Is(err, ErrDeviceNotReady), errors.Is(err, ErrDeviceIDRequired),
		errors.Is(err, ErrNameRequired), errors.Is(err, ErrInvalidCategory),
		errors.Is(err, ErrInvalidLanguage), errors.Is(err, ErrComponentsEmpty):
		return response.BadRequest(c, err.Error())
	default:
		if strings.Contains(err.Error(), "meta create template error") {
			return response.BadRequest(c, err.Error())
		}
		return response.InternalError(c, "internal server error")
	}
}
