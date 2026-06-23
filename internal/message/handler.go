package message

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
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

	page := repository.PageParams{
		Page:     c.QueryInt("page", 1),
		PageSize: c.QueryInt("limit", 50),
	}
	result, err := h.service.ListByConversation(c.Context(), claims.TeamID, claims.UserID, string(claims.Role), conversationID, ListFilter{}, page)
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "ok", fiber.Map{"messages": result.Items, "pagination": result.Pagination})
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

	msg, err := h.service.Create(c.Context(), claims.TeamID, claims.UserID, string(claims.Role), CreateInput{
		ConversationID: conversationID,
		Direction:      DirectionOutbound,
		Sender:         claims.UserID.String(),
		Content:        req.Content,
	})
	if err != nil {
		return h.mapError(c, err)
	}
	return response.Created(c, "message sent", msg)
}

func (h *Handler) mapError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, serviceerrors.ErrForbidden):
		return response.Fail(c, fiber.StatusForbidden, err.Error())
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, team.ErrMemberNotFound):
		return response.NotFound(c, err.Error())
	default:
		if serviceerrors.IsValidation(err) {
			return response.BadRequest(c, err.Error())
		}
		return response.InternalError(c, err.Error())
	}
}
