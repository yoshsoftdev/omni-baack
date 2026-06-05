package message

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/team"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type sendMessageRequest struct {
	Content string `json:"content"`
}

func (h *Handler) List(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	conversationID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid conversation id")
	}

	messages, err := h.service.List(c.Context(), claims.TeamID, claims.UserID, conversationID)
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "ok", fiber.Map{"messages": messages})
}

func (h *Handler) Send(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	conversationID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid conversation id")
	}

	var req sendMessageRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	msg, err := h.service.Send(c.Context(), claims.TeamID, claims.UserID, conversationID, SendInput{
		Content: req.Content,
	})
	if err != nil {
		return h.mapError(c, err)
	}
	return response.Created(c, "message sent", msg)
}

func (h *Handler) mapError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrInvalidContent), errors.Is(err, ErrDeviceNotConnected):
		return response.BadRequest(c, err.Error())
	case errors.Is(err, ErrDeviceNotFound), errors.Is(err, ErrConversationNotFound), errors.Is(err, team.ErrMemberNotFound):
		return response.NotFound(c, err.Error())
	default:
		return response.InternalError(c, err.Error())
	}
}
