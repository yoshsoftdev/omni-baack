package campaign

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

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

type createRequest struct {
	Name            string          `json:"name"`
	DeviceID        string          `json:"device_id"`
	TemplateID      string          `json:"template_id"`
	VariableMapping json.RawMessage `json:"variable_mapping"`
	ContactIDs      []string        `json:"contact_ids"`
	TagIDs          []string        `json:"tag_ids"`
	ScheduledAt     *string         `json:"scheduled_at"`
}

type sendRequest struct {
	ContactIDs []string `json:"contact_ids"`
}

func (h *Handler) List(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	items, err := h.service.List(c.Context(), claims.TeamID, claims.UserID)
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "ok", fiber.Map{"campaigns": items})
}

func (h *Handler) Create(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	var req createRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	deviceID, err := uuid.Parse(strings.TrimSpace(req.DeviceID))
	if err != nil {
		return response.BadRequest(c, "invalid device_id")
	}
	templateID, err := uuid.Parse(strings.TrimSpace(req.TemplateID))
	if err != nil {
		return response.BadRequest(c, "invalid template_id")
	}

	var scheduledAt *time.Time
	if req.ScheduledAt != nil && strings.TrimSpace(*req.ScheduledAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.ScheduledAt))
		if err != nil {
			return response.BadRequest(c, "invalid scheduled_at")
		}
		scheduledAt = &parsed
	}

	item, err := h.service.Create(c.Context(), claims.TeamID, claims.UserID, team.Role(claims.Role), CreateInput{
		Name:            strings.TrimSpace(req.Name),
		DeviceID:        deviceID,
		TemplateID:      templateID,
		VariableMapping: req.VariableMapping,
		ContactIDs:      parseUUIDs(req.ContactIDs),
		TagIDs:          parseUUIDs(req.TagIDs),
		ScheduledAt:     scheduledAt,
	})
	if err != nil {
		return h.mapError(c, err)
	}
	return response.Created(c, "campaign created", item)
}

func (h *Handler) Send(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	campaignID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid campaign id")
	}

	var req sendRequest
	_ = c.BodyParser(&req)

	item, err := h.service.Send(c.Context(), claims.TeamID, claims.UserID, team.Role(claims.Role), campaignID, parseUUIDs(req.ContactIDs))
	if err != nil {
		return h.mapError(c, err)
	}
	return response.Accepted(c, "campaign queued", item)
}

func (h *Handler) Report(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	campaignID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid campaign id")
	}

	report, err := h.service.GetReport(c.Context(), claims.TeamID, claims.UserID, campaignID)
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "ok", report)
}

func (h *Handler) mapError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, team.ErrMemberNotFound):
		return response.Forbidden(c, err.Error())
	case errors.Is(err, ErrForbidden):
		return response.Forbidden(c, err.Error())
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrTemplateNotFound), errors.Is(err, ErrDeviceNotFound):
		return response.NotFound(c, err.Error())
	case errors.Is(err, ErrNameRequired), errors.Is(err, ErrRecipientsRequired),
		errors.Is(err, ErrInvalidStatus), errors.Is(err, ErrDeviceNotConnected):
		return response.BadRequest(c, err.Error())
	default:
		if strings.Contains(err.Error(), "variable_mapping") {
			return response.BadRequest(c, err.Error())
		}
		return response.InternalError(c, "internal server error")
	}
}

func parseUUIDs(values []string) []uuid.UUID {
	if len(values) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(values))
	for _, raw := range values {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}
