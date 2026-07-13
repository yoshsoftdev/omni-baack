package handler

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

type InboxHandler struct {
	list *inbox.ListService
}

func NewInboxHandler(list *inbox.ListService) *InboxHandler {
	return &InboxHandler{list: list}
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
		Cursor: strings.TrimSpace(c.Query("cursor")),
		Limit:  limit,
	})
	if err != nil {
		if strings.Contains(err.Error(), "invalid cursor") {
			return response.BadRequest(c, "invalid cursor")
		}
		if serviceerrors.IsValidation(err) {
			return response.BadRequest(c, err.Error())
		}
		return mapError(c, err)
	}

	return response.OK(c, "ok", result)
}
