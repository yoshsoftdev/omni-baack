package handler

import (
	"strings"

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
	Content         string `json:"content"`
	StorageObjectID string `json:"storage_object_id"`
	ReplyToWAMID    string `json:"reply_to_wamid"`
	ClientMessageID string `json:"client_message_id"`
}

type deleteMessageRequest struct {
	PropagateToWA bool `json:"propagate_to_wa"`
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
	sendInput := message.SendInput{
		Content:      req.Content,
		ReplyToWAMID: strings.TrimSpace(req.ReplyToWAMID),
	}
	if strings.TrimSpace(req.StorageObjectID) != "" {
		objectID, parseErr := parseUUIDFromString(req.StorageObjectID)
		if parseErr != nil {
			return response.BadRequest(c, "invalid storage_object_id")
		}
		sendInput.StorageObjectID = &objectID
	}
	if strings.TrimSpace(req.ClientMessageID) != "" {
		clientID, parseErr := parseUUIDFromString(req.ClientMessageID)
		if parseErr != nil {
			return response.BadRequest(c, "invalid client_message_id")
		}
		sendInput.ClientMessageID = &clientID
	}
	conv, err := h.conversations.Get(c.Context(), actor.TeamID, actor.UserID, actor.Role, conversationID)
	if err != nil {
		return mapError(c, err)
	}
	msg, err := h.svc.SendMessage(c.Context(), actor.TeamID, actor.UserID, actor.Role, conversationID, sendInput)
	if err != nil {
		return mapError(c, err)
	}
	if h.inbox != nil {
		_, _ = h.inbox.HandleOutbound(c.Context(), inbox.OutboundParams{
			TeamID:       actor.TeamID,
			AgentID:      actor.UserID,
			Conversation: &conv.Conversation,
			Message:      msg,
		})
	}
	return response.Created(c, "message sent", msg)
}

func (h *MessageHandler) Delete(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	messageID, err := parseUUIDParam(c, "messageId")
	if err != nil {
		return response.BadRequest(c, "invalid message id")
	}
	var req deleteMessageRequest
	_ = c.BodyParser(&req)
	_ = req // propagate handled by service layer fallback in current phase

	msg, err := h.svc.DeleteByID(c.Context(), actor.TeamID, actor.UserID, actor.Role, messageID, message.DeletedBySourceAgent)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "message deleted", msg)
}
