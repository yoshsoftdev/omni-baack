package broadcast

import (
	"errors"
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

type createBroadcastRequest struct {
	Name        string   `json:"name"`
	Message     string   `json:"message"`
	DeviceID    string   `json:"device_id"`
	ContactIDs  []string `json:"contact_ids"`
	TagIDs      []string `json:"tag_ids"`
	ScheduledAt *string  `json:"scheduled_at"`
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
	return response.OK(c, "ok", fiber.Map{"broadcasts": items})
}

func (h *Handler) Get(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	broadcastID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid broadcast id")
	}

	item, err := h.service.Get(c.Context(), claims.TeamID, claims.UserID, broadcastID)
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "ok", item)
}

func (h *Handler) Create(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	var req createBroadcastRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	deviceID, err := uuid.Parse(req.DeviceID)
	if err != nil {
		return response.BadRequest(c, "invalid device id")
	}

	item, err := h.service.Create(c.Context(), claims.TeamID, claims.UserID, team.Role(claims.Role), CreateInput{
		Name:        req.Name,
		Message:     req.Message,
		DeviceID:    deviceID,
		ContactIDs:  parseUUIDs(req.ContactIDs),
		TagIDs:      parseUUIDs(req.TagIDs),
		ScheduledAt: parseTime(req.ScheduledAt),
	})
	if err != nil {
		return h.mapError(c, err)
	}
	return response.Created(c, "broadcast created", item)
}

func (h *Handler) Cancel(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	broadcastID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid broadcast id")
	}

	item, err := h.service.Cancel(c.Context(), claims.TeamID, claims.UserID, team.Role(claims.Role), broadcastID)
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "broadcast cancelled", item)
}

func (h *Handler) mapError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrForbidden):
		return response.Fail(c, fiber.StatusForbidden, err.Error())
	case errors.Is(err, ErrNameRequired), errors.Is(err, ErrMessageRequired),
		errors.Is(err, ErrRecipientsRequired), errors.Is(err, ErrDeviceNotConnected),
		errors.Is(err, ErrCannotCancel):
		return response.BadRequest(c, err.Error())
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrDeviceNotFound), errors.Is(err, team.ErrMemberNotFound):
		return response.NotFound(c, err.Error())
	default:
		return response.InternalError(c, "something went wrong")
	}
}

func parseUUIDs(values []string) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		id, err := uuid.Parse(value)
		if err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func parseTime(raw *string) *time.Time {
	if raw == nil || *raw == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, *raw)
	if err != nil {
		return nil
	}
	return &parsed
}
