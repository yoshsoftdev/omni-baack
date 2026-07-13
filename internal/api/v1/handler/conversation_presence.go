package handler

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/auth"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
	"github.com/omnichannel/omni-chan-v2/internal/user"
)

type ConversationPresenceHandler struct {
	collisions    *inbox.CollisionService
	conversations *conversation.Service
	users         user.Repository
}

func NewConversationPresenceHandler(
	collisions *inbox.CollisionService,
	conversations *conversation.Service,
	users user.Repository,
) *ConversationPresenceHandler {
	return &ConversationPresenceHandler{
		collisions:    collisions,
		conversations: conversations,
		users:         users,
	}
}

type typingPresenceRequest struct {
	Action string `json:"action"`
}

func (h *ConversationPresenceHandler) Open(c *fiber.Ctx) error {
	actor, conversationID, agentName, err := h.prepare(c)
	if err != nil {
		return err
	}
	snapshot, err := h.collisions.OpenConversation(c.Context(), actor.TeamID, conversationID, actor.UserID, agentName)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", snapshot)
}

func (h *ConversationPresenceHandler) Close(c *fiber.Ctx) error {
	actor, conversationID, agentName, err := h.prepare(c)
	if err != nil {
		return err
	}
	snapshot, err := h.collisions.CloseConversation(c.Context(), actor.TeamID, conversationID, actor.UserID, agentName)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", snapshot)
}

func (h *ConversationPresenceHandler) Typing(c *fiber.Ctx) error {
	actor, conversationID, agentName, err := h.prepare(c)
	if err != nil {
		return err
	}
	var req typingPresenceRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	var snapshot inbox.CollisionSnapshot
	switch action {
	case "start":
		snapshot, err = h.collisions.StartTyping(c.Context(), actor.TeamID, conversationID, actor.UserID, agentName)
	case "stop":
		snapshot, err = h.collisions.StopTyping(c.Context(), actor.TeamID, conversationID, actor.UserID, agentName)
	default:
		return response.BadRequest(c, "action must be start or stop")
	}
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", snapshot)
}

func (h *ConversationPresenceHandler) Get(c *fiber.Ctx) error {
	_, conversationID, _, err := h.prepare(c)
	if err != nil {
		return err
	}
	snapshot, err := h.collisions.GetSnapshot(c.Context(), conversationID)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", snapshot)
}

func (h *ConversationPresenceHandler) prepare(c *fiber.Ctx) (auth.Actor, uuid.UUID, string, error) {
	actor, err := requireActor(c)
	if err != nil {
		return auth.Actor{}, uuid.Nil, "", err
	}
	conversationID, err := parseUUIDParam(c, "id")
	if err != nil {
		return auth.Actor{}, uuid.Nil, "", response.BadRequest(c, "invalid conversation id")
	}
	if _, err := h.conversations.Get(c.Context(), actor.TeamID, actor.UserID, actor.Role, conversationID); err != nil {
		return auth.Actor{}, uuid.Nil, "", mapError(c, err)
	}
	agentName := actor.Email
	if h.users != nil {
		if u, userErr := h.users.FindByID(c.Context(), actor.UserID); userErr == nil && u != nil {
			if name := strings.TrimSpace(u.Name); name != "" {
				agentName = name
			}
		}
	}
	return actor, conversationID, agentName, nil
}
