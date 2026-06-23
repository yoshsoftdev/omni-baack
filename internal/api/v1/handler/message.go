package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/api/v1/dto"
	"github.com/omnichannel/omni-chan-v2/internal/message"
)

type MessageHandler struct {
	svc *message.Service
}

func NewMessageHandler(svc *message.Service) *MessageHandler {
	return &MessageHandler{svc: svc}
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
	msg, err := h.svc.Create(c.Context(), actor.TeamID, actor.UserID, actor.Role, message.CreateInput{
		ConversationID: conversationID,
		Direction:      message.DirectionOutbound,
		Sender:         actor.UserID.String(),
		Content:        req.Content,
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.Created(c, "message sent", msg)
}
