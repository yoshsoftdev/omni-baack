package team

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type updateTeamRequest struct {
	Name string `json:"name"`
}

type addMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type updateMemberRoleRequest struct {
	Role string `json:"role"`
}

func (h *Handler) Get(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	team, err := h.service.Get(c.Context(), claims.TeamID, claims.UserID)
	if err != nil {
		return h.mapError(c, err)
	}

	return response.OK(c, "ok", toTeamResponse(team))
}

func (h *Handler) Update(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	var req updateTeamRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	team, err := h.service.Update(c.Context(), claims.TeamID, claims.UserID, Role(claims.Role), req.Name)
	if err != nil {
		return h.mapError(c, err)
	}

	return response.OK(c, "team updated", toTeamResponse(team))
}

func (h *Handler) ListMembers(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	members, err := h.service.ListMembers(c.Context(), claims.TeamID, claims.UserID)
	if err != nil {
		return h.mapError(c, err)
	}

	return response.OK(c, "ok", fiber.Map{"members": members})
}

func (h *Handler) AddMember(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	var req addMemberRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if req.Email == "" || req.Role == "" {
		return response.BadRequest(c, "email and role are required")
	}

	member, err := h.service.AddMember(
		c.Context(),
		claims.TeamID,
		claims.UserID,
		Role(claims.Role),
		req.Email,
		Role(req.Role),
	)
	if err != nil {
		return h.mapError(c, err)
	}

	return response.Created(c, "member added", member)
}

func (h *Handler) UpdateMemberRole(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	memberID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid member id")
	}

	var req updateMemberRoleRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if req.Role == "" {
		return response.BadRequest(c, "role is required")
	}

	member, err := h.service.UpdateMemberRole(
		c.Context(),
		claims.TeamID,
		claims.UserID,
		Role(claims.Role),
		memberID,
		Role(req.Role),
	)
	if err != nil {
		return h.mapError(c, err)
	}

	return response.OK(c, "member updated", member)
}

func (h *Handler) RemoveMember(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	memberID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid member id")
	}

	if err := h.service.RemoveMember(c.Context(), claims.TeamID, claims.UserID, Role(claims.Role), memberID); err != nil {
		return h.mapError(c, err)
	}

	return response.OK(c, "member removed", nil)
}

func (h *Handler) mapError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrForbidden):
		return response.Fail(c, fiber.StatusForbidden, err.Error())
	case errors.Is(err, ErrInvalidRole), errors.Is(err, ErrAlreadyMember):
		return response.BadRequest(c, err.Error())
	case errors.Is(err, ErrCannotModifyOwner):
		return response.BadRequest(c, err.Error())
	case errors.Is(err, ErrUserNotFound), errors.Is(err, ErrMemberNotFound), errors.Is(err, ErrNotFound):
		return response.NotFound(c, err.Error())
	default:
		if err.Error() == "team name is required" {
			return response.BadRequest(c, err.Error())
		}
		return response.InternalError(c, "something went wrong")
	}
}

func toTeamResponse(team *Team) fiber.Map {
	return fiber.Map{
		"id":   team.ID,
		"name": team.Name,
		"slug": team.Slug,
	}
}
