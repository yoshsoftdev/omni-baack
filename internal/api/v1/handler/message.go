package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/api/v1/dto"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
	"github.com/omnichannel/omni-chan-v2/internal/message"
)

type MessageHandler struct {
	svc           *message.Service
	conversations *conversation.Service
	inbox         *inbox.SyncService
}

func NewMessageHandler(svc *message.Service, conversations *conversation.Service, inboxSync *inbox.SyncService) *MessageHandler {
	return &MessageHandler{svc: svc, conversations: conversations, inbox: inboxSync}
}

type sendMessageRequest struct {
	Content string `json:"content"`
}

func (h *MessageHandler) List(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	conversationID, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid conversation id")
	}
	params, err := pageParams(c)
	if err != nil {
		return mapError(c, err)
	}
	page, err := h.svc.ListByConversation(c.Context(), actor.TeamID, actor.UserID, actor.Role, conversationID, message.ListFilter{}, params)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", dto.NewPageResponse(page))
}

func (h *MessageHandler) Send(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	conversationID, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid conversation id")
	}
	var req sendMessageRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	conv, err := h.conversations.Get(c.Context(), actor.TeamID, actor.UserID, actor.Role, conversationID)
	if err != nil {
		return mapError(c, err)
	}
	msg, err := h.svc.SendReply(c.Context(), actor.TeamID, actor.UserID, actor.Role, conversationID, req.Content)
	if err != nil {
		return mapError(c, err)
	}
	if h.inbox != nil {
		_, _ = h.inbox.HandleOutbound(c.Context(), inbox.OutboundParams{
			TeamID:       actor.TeamID,
			AgentID:      actor.UserID,
			Conversation: conv,
			Message:      msg,
		})
	}
	return response.Created(c, "message sent", msg)
}
