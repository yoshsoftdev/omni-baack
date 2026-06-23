package conversation

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

	page := repository.PageParams{
		Page:     c.QueryInt("page", 1),
		PageSize: c.QueryInt("limit", 50),
	}
	result, err := h.service.List(c.Context(), claims.TeamID, claims.UserID, string(claims.Role), ListFilter{
		Status:     status,
		AssignedTo: assignedTo,
		Search:     c.Query("search"),
	}, page)
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "ok", fiber.Map{"conversations": result.Items, "pagination": result.Pagination})
}

func (h *Handler) UnreadCount(c *fiber.Ctx) error {
	_, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	return response.OK(c, "ok", fiber.Map{"unread_count": 0})
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

	item, err := h.service.Get(c.Context(), claims.TeamID, claims.UserID, string(claims.Role), conversationID)
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

	item, err := h.service.Assign(c.Context(), claims.TeamID, claims.UserID, string(claims.Role), conversationID, assigneeID)
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

	item, err := h.service.UpdateStatus(c.Context(), claims.TeamID, claims.UserID, string(claims.Role), conversationID, Status(req.Status))
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "conversation updated", item)
}

func (h *Handler) MarkRead(c *fiber.Ctx) error {
	return response.Fail(c, fiber.StatusNotImplemented, "mark read pending service migration")
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
		return response.InternalError(c, "something went wrong")
	}
}
