package conversation

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

type assignRequest struct {
	UserID *string `json:"user_id"`
}

type statusRequest struct {
	Status string `json:"status"`
}

func (h *Handler) List(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	var status *Status
	if raw := c.Query("status"); raw != "" {
		s := Status(raw)
		status = &s
	}
	var assignedTo *uuid.UUID
	if raw := c.Query("assigned_to"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return response.BadRequest(c, "invalid assigned_to")
		}
		assignedTo = &id
	}

	items, err := h.service.List(c.Context(), claims.TeamID, claims.UserID, ListInput{
		Status:     status,
		AssignedTo: assignedTo,
		Search:     c.Query("search"),
		Limit:      c.QueryInt("limit", 50),
	})
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "ok", fiber.Map{"conversations": items})
}

func (h *Handler) Get(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	conversationID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid conversation id")
	}

	item, err := h.service.Get(c.Context(), claims.TeamID, claims.UserID, conversationID)
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "ok", item)
}

func (h *Handler) Assign(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	conversationID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid conversation id")
	}

	var req assignRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	var assigneeID *uuid.UUID
	if req.UserID != nil && *req.UserID != "" {
		id, err := uuid.Parse(*req.UserID)
		if err != nil {
			return response.BadRequest(c, "invalid user id")
		}
		assigneeID = &id
	}

	item, err := h.service.Assign(c.Context(), claims.TeamID, claims.UserID, team.Role(claims.Role), conversationID, assigneeID)
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "conversation assigned", item)
}

func (h *Handler) UpdateStatus(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	conversationID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid conversation id")
	}

	var req statusRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	item, err := h.service.UpdateStatus(c.Context(), claims.TeamID, claims.UserID, conversationID, Status(req.Status))
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "conversation updated", item)
}

func (h *Handler) MarkRead(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	conversationID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid conversation id")
	}

	item, err := h.service.MarkRead(c.Context(), claims.TeamID, claims.UserID, conversationID)
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "conversation marked read", item)
}

func (h *Handler) mapError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrForbidden):
		return response.Fail(c, fiber.StatusForbidden, err.Error())
	case errors.Is(err, ErrInvalidStatus), errors.Is(err, ErrAssigneeNotFound):
		return response.BadRequest(c, err.Error())
	case errors.Is(err, ErrNotFound), errors.Is(err, team.ErrMemberNotFound):
		return response.NotFound(c, err.Error())
	default:
		return response.InternalError(c, "something went wrong")
	}
}
