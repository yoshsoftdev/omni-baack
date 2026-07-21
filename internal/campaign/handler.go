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
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
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
	ChannelID       string          `json:"channel_id"`
	TemplateID      string          `json:"template_id"`
	VariableMapping json.RawMessage `json:"variable_mapping"`
	SegmentID       string          `json:"segment_id"`
	SegmentIDs      []string        `json:"segment_ids"`
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

	page := repository.PageParams{
		Page:     c.QueryInt("page", 1),
		PageSize: c.QueryInt("limit", 50),
	}
	result, err := h.service.List(c.Context(), claims.TeamID, claims.UserID, string(claims.Role), ListFilter{
		Search: strings.TrimSpace(c.Query("search")),
	}, page)
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "ok", fiber.Map{"campaigns": result.Items, "pagination": result.Pagination})
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

	channelRaw := strings.TrimSpace(req.ChannelID)
	if channelRaw == "" {
		channelRaw = strings.TrimSpace(req.DeviceID)
	}
	channelID, err := uuid.Parse(channelRaw)
	if err != nil {
		return response.BadRequest(c, "invalid channel_id")
	}
	templateID, err := uuid.Parse(strings.TrimSpace(req.TemplateID))
	if err != nil {
		return response.BadRequest(c, "invalid template_id")
	}

	var scheduledAt *time.Time
	sendMode := SendNow
	if req.ScheduledAt != nil && strings.TrimSpace(*req.ScheduledAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.ScheduledAt))
		if err != nil {
			return response.BadRequest(c, "invalid scheduled_at")
		}
		scheduledAt = &parsed
		sendMode = SendLater
	}

	segmentIDs := parseUUIDs(req.SegmentIDs)
	if len(segmentIDs) == 0 && strings.TrimSpace(req.SegmentID) != "" {
		if id, parseErr := uuid.Parse(strings.TrimSpace(req.SegmentID)); parseErr == nil {
			segmentIDs = append(segmentIDs, id)
		}
	}
	if len(segmentIDs) > 0 {
		return response.BadRequest(c, "segment audiences are no longer supported; use tag_ids or contact_ids")
	}

	audiences := make([]AudienceInput, 0, len(req.ContactIDs)+len(req.TagIDs))
	for _, id := range parseUUIDs(req.ContactIDs) {
		audiences = append(audiences, AudienceInput{SourceType: AudienceContact, SourceID: id})
	}
	for _, id := range parseUUIDs(req.TagIDs) {
		audiences = append(audiences, AudienceInput{SourceType: AudienceTag, SourceID: id})
	}

	result, err := h.service.Create(c.Context(), claims.TeamID, claims.UserID, string(claims.Role), CreateInput{
		Name:            strings.TrimSpace(req.Name),
		ChannelID:       channelID,
		TemplateID:      templateID,
		SegmentID:       nil,
		Audiences:       audiences,
		VariableMapping: req.VariableMapping,
		SendMode:        sendMode,
		ScheduledAt:     scheduledAt,
	})
	if err != nil {
		return h.mapError(c, err)
	}
	return response.Created(c, "campaign created", result)
}

func (h *Handler) Send(c *fiber.Ctx) error {
	return response.Fail(c, fiber.StatusNotImplemented, "campaign send pending service migration")
}

func (h *Handler) Overview(c *fiber.Ctx) error {
	return response.Fail(c, fiber.StatusNotImplemented, "campaign overview pending service migration")
}

func (h *Handler) Cancel(c *fiber.Ctx) error {
	return response.Fail(c, fiber.StatusNotImplemented, "campaign cancel pending service migration")
}

func (h *Handler) Report(c *fiber.Ctx) error {
	return response.Fail(c, fiber.StatusNotImplemented, "campaign report pending service migration")
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
