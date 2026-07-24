package handler

import (
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/api/v1/dto"
	"github.com/omnichannel/omni-chan-v2/internal/template"
)

type TemplateHandler struct {
	svc *template.Service
}

func NewTemplateHandler(svc *template.Service) *TemplateHandler {
	return &TemplateHandler{svc: svc}
}

type createTemplateRequest struct {
	DeviceID   string          `json:"device_id"`
	ChannelID  string          `json:"channel_id"`
	Name       string          `json:"name"`
	Language   string          `json:"language"`
	Category   string          `json:"category"`
	Components json.RawMessage `json:"components"`
}

func (h *TemplateHandler) List(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	channelID, err := optionalUUIDQuery(c, "channel_id")
	if err != nil {
		return response.BadRequest(c, "invalid channel_id")
	}
	var status *template.Status
	if raw := strings.TrimSpace(c.Query("status")); raw != "" {
		s := template.Status(raw)
		status = &s
	}
	params, err := pageParams(c)
	if err != nil {
		return mapError(c, err)
	}
	page, err := h.svc.List(c.Context(), actor.TeamID, actor.UserID, actor.Role, template.ListFilter{
		Search:    strings.TrimSpace(c.Query("search")),
		Status:    status,
		ChannelID: channelID,
	}, params)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", dto.NewPageResponse(page))
}

func (h *TemplateHandler) Get(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid template id")
	}
	tpl, err := h.svc.Get(c.Context(), actor.TeamID, actor.UserID, actor.Role, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", tpl)
}

func (h *TemplateHandler) Create(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	var req createTemplateRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	rawChannelID := strings.TrimSpace(req.ChannelID)
	if rawChannelID == "" {
		rawChannelID = strings.TrimSpace(req.DeviceID)
	}
	channelID, err := uuid.Parse(rawChannelID)
	if err != nil {
		return response.BadRequest(c, "channel_id (or device_id) is required")
	}

	tpl, err := h.svc.Create(c.Context(), actor.TeamID, actor.UserID, actor.Role, template.CreateInput{
		ChannelID:  channelID,
		Name:       req.Name,
		Language:   req.Language,
		Category:   req.Category,
		Components: req.Components,
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.Created(c, "template submitted to Meta for review", tpl)
}

func (h *TemplateHandler) Sync(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	channelID, err := optionalUUIDQuery(c, "channel_id")
	if err != nil || channelID == nil {
		return response.BadRequest(c, "channel_id is required")
	}
	result, err := h.svc.SyncFromMeta(c.Context(), actor.TeamID, actor.UserID, actor.Role, *channelID)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "templates synced", result)
}

func (h *TemplateHandler) SyncHistory(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	params, err := pageParams(c)
	if err != nil {
		return mapError(c, err)
	}
	channelID, err := optionalUUIDQuery(c, "channel_id")
	if err != nil {
		return response.BadRequest(c, "invalid channel_id")
	}
	filter := template.SyncRunListFilter{ChannelID: channelID}
	page, err := h.svc.ListSyncHistory(c.Context(), actor.TeamID, actor.UserID, actor.Role, filter, params)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", dto.NewPageResponse(page))
}

func (h *TemplateHandler) GetSyncHistory(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid sync history id")
	}
	item, err := h.svc.GetSyncHistory(c.Context(), actor.TeamID, actor.UserID, actor.Role, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", item)
}

func (h *TemplateHandler) Delete(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid template id")
	}
	if err := h.svc.Delete(c.Context(), actor.TeamID, actor.UserID, actor.Role, id); err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "template deleted", nil)
}
