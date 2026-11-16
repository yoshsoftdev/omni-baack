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
	autoReply       *inbox.AutoReplyService
}

func NewInboxHandler(list *inbox.ListService, windowRecontact *inbox.WindowRecontactService, autoReply *inbox.AutoReplyService) *InboxHandler {
	return &InboxHandler{list: list, windowRecontact: windowRecontact, autoReply: autoReply}
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

type welcomeRequest struct {
	Enabled bool   `json:"enabled"`
	Text    string `json:"text"`
}

type keywordRuleRequest struct {
	Keywords  []string `json:"keywords"`
	MatchType string   `json:"match_type"`
	ReplyText string   `json:"reply_text"`
	Enabled   *bool    `json:"enabled"`
	SortOrder int      `json:"sort_order"`
}

func (r keywordRuleRequest) rule() inbox.KeywordRule {
	enabled := true
	if r.Enabled != nil {
		enabled = *r.Enabled
	}
	return inbox.KeywordRule{Keywords: r.Keywords, MatchType: r.MatchType, ReplyText: r.ReplyText, Enabled: enabled, SortOrder: r.SortOrder}
}

// GetWelcomeSettings returns the welcome message sent on a customer's first conversation (9.2).
func (h *InboxHandler) GetWelcomeSettings(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	settings, err := h.autoReply.GetWelcome(c.Context(), actor.TeamID, actor.UserID, actor.Role)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", settings)
}

func (h *InboxHandler) UpdateWelcomeSettings(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	var req welcomeRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	settings, err := h.autoReply.UpdateWelcome(c.Context(), actor.TeamID, actor.UserID, actor.Role, inbox.WelcomeSettings{Enabled: req.Enabled, Text: req.Text})
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "welcome message updated", settings)
}

func (h *InboxHandler) ListKeywordRules(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	rules, err := h.autoReply.ListRules(c.Context(), actor.TeamID, actor.UserID, actor.Role)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", fiber.Map{"items": rules})
}

func (h *InboxHandler) CreateKeywordRule(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	var req keywordRuleRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	rule, err := h.autoReply.CreateRule(c.Context(), actor.TeamID, actor.UserID, actor.Role, req.rule())
	if err != nil {
		return mapError(c, err)
	}
	return response.Created(c, "keyword rule created", rule)
}

func (h *InboxHandler) UpdateKeywordRule(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid rule id")
	}
	var req keywordRuleRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	rule, err := h.autoReply.UpdateRule(c.Context(), actor.TeamID, actor.UserID, actor.Role, id, req.rule())
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "keyword rule updated", rule)
}

func (h *InboxHandler) DeleteKeywordRule(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid rule id")
	}
	if err := h.autoReply.DeleteRule(c.Context(), actor.TeamID, actor.UserID, actor.Role, id); err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "keyword rule deleted", nil)
}
