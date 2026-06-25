package handler

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/api/v1/dto"
	"github.com/omnichannel/omni-chan-v2/internal/conversation"
)

type ConversationHandler struct {
	svc *conversation.Service
}

func NewConversationHandler(svc *conversation.Service) *ConversationHandler {
	return &ConversationHandler{svc: svc}
}

type createConversationRequest struct {
	ContactID   string `json:"contact_id"`
	ChannelID   string `json:"channel_id"`
	ChannelType string `json:"channel_type"`
}

type assignConversationRequest struct {
	UserID *string `json:"user_id"`
}

type updateConversationStatusRequest struct {
	Status string `json:"status"`
}

func (h *ConversationHandler) List(c *fiber.Ctx) error {
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
	view := conversation.InboxView(strings.TrimSpace(c.Query("view")))
	params, err := pageParams(c)
	if err != nil {
		return mapError(c, err)
	}
	page, err := h.svc.List(c.Context(), actor.TeamID, actor.UserID, actor.Role, conversation.ListFilter{
		Status:     status,
		AssignedTo: assignedTo,
		Search:     strings.TrimSpace(c.Query("search")),
		View:       view,
	}, params)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", dto.NewPageResponse(page))
}

func (h *ConversationHandler) Get(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid conversation id")
	}
	conv, err := h.svc.Get(c.Context(), actor.TeamID, actor.UserID, actor.Role, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", conv)
}

func (h *ConversationHandler) Create(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	var req createConversationRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	contactID, err := parseUUIDFromString(req.ContactID)
	if err != nil {
		return response.BadRequest(c, "invalid contact_id")
	}
	var channelID *uuid.UUID
	if strings.TrimSpace(req.ChannelID) != "" {
		parsed, err := parseUUIDFromString(req.ChannelID)
		if err != nil {
			return response.BadRequest(c, "invalid channel_id")
		}
		channelID = &parsed
	}
	conv, err := h.svc.Create(c.Context(), actor.TeamID, actor.UserID, actor.Role, conversation.CreateInput{
		ContactID:   contactID,
		ChannelID:   channelID,
		ChannelType: conversation.ChannelType(req.ChannelType),
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.Created(c, "conversation created", conv)
}

func (h *ConversationHandler) Assign(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid conversation id")
	}
	var req assignConversationRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	var assigneeID *uuid.UUID
	if req.UserID != nil && strings.TrimSpace(*req.UserID) != "" {
		parsed, err := parseUUIDFromString(*req.UserID)
		if err != nil {
			return response.BadRequest(c, "invalid user_id")
		}
		assigneeID = &parsed
	}
	conv, err := h.svc.Assign(c.Context(), actor.TeamID, actor.UserID, actor.Role, id, assigneeID)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "conversation assigned", conv)
}

func (h *ConversationHandler) UpdateStatus(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid conversation id")
	}
	var req updateConversationStatusRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	conv, err := h.svc.UpdateStatus(c.Context(), actor.TeamID, actor.UserID, actor.Role, id, conversation.Status(req.Status))
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "conversation updated", conv)
}

func (h *ConversationHandler) MarkRead(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid conversation id")
	}
	conv, err := h.svc.MarkRead(c.Context(), actor.TeamID, actor.UserID, actor.Role, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "conversation marked read", conv)
}
