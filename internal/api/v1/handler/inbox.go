package handler

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
)

type InboxHandler struct {
	list            *inbox.ListService
	windowRecontact *inbox.WindowRecontactService
}

func NewInboxHandler(list *inbox.ListService, windowRecontact *inbox.WindowRecontactService) *InboxHandler {
	return &InboxHandler{list: list, windowRecontact: windowRecontact}
}

func (h *InboxHandler) List(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	var status *conversation.Status
	if raw := strings.TrimSpace(c.Query("status")); raw != "" {
		s := conversation.Status(raw)
		status = &s
	}
	assignedTo, err := optionalUUIDQuery(c, "assigned_to")
	if err != nil {
		return response.BadRequest(c, "invalid assigned_to")
	}
	channelID, err := optionalUUIDQuery(c, "channel_id")
	if err != nil {
		return response.BadRequest(c, "invalid channel_id")
	}
	var channelType *conversation.ChannelType
	if raw := strings.TrimSpace(c.Query("channel_type")); raw != "" && raw != "all" {
		ct := conversation.ChannelType(raw)
		channelType = &ct
	}

	limit := c.QueryInt("limit", 50)
	if limit < 1 || limit > 100 {
		return response.BadRequest(c, "limit must be between 1 and 100")
	}

	view := conversation.InboxView(strings.TrimSpace(c.Query("view")))
	result, err := h.list.List(c.Context(), actor.TeamID, actor.UserID, actor.Role, inbox.ListInput{
		Filter: inbox.ListFilterInput{
			Status:      status,
			AssignedTo:  assignedTo,
			ChannelID:   channelID,
			ChannelType: channelType,
			Search:      strings.TrimSpace(c.Query("search")),
			View:        view,
		},
		Limit:  limit,
		Cursor: strings.TrimSpace(c.Query("cursor")),
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", result)
}

type updateWindowRecontactRequest struct {
	Enabled    *bool   `json:"enabled"`
	Text       *string `json:"text"`
	AfterHours *int    `json:"after_hours"`
}

func (h *InboxHandler) GetWindowRecontactSettings(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	if h.windowRecontact == nil {
		return response.OK(c, "ok", inbox.DefaultWindowRecontactSettings())
	}
	settings, err := h.windowRecontact.GetSettings(c.Context(), actor.TeamID, actor.UserID, actor.Role)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", settings)
}

func (h *InboxHandler) UpdateWindowRecontactSettings(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	if h.windowRecontact == nil {
		return response.BadRequest(c, "window recontact is not configured")
	}

	current, err := h.windowRecontact.GetSettings(c.Context(), actor.TeamID, actor.UserID, actor.Role)
	if err != nil {
		return mapError(c, err)
	}

	var req updateWindowRecontactRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	next := current
	if req.Enabled != nil {
		next.Enabled = *req.Enabled
	}
	if req.Text != nil {
		next.Text = *req.Text
	}
	if req.AfterHours != nil {
		next.AfterHours = *req.AfterHours
	}

	updated, err := h.windowRecontact.UpdateSettings(c.Context(), actor.TeamID, actor.UserID, actor.Role, next)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "window recontact settings updated", updated)
}
