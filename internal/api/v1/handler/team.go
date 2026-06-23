package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	"github.com/omnichannel/omni-chan-v2/internal/team"
)

type TeamHandler struct {
	svc  *team.Service
	rbac *rbac.Service
}

func NewTeamHandler(svc *team.Service, rbacSvc *rbac.Service) *TeamHandler {
	return &TeamHandler{svc: svc, rbac: rbacSvc}
}

type updateTeamRequest struct {
	Name string `json:"name"`
}

type addMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type inviteUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type updateRoleRequest struct {
	Role string `json:"role"`
}

func (h *TeamHandler) Get(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	t, err := h.svc.Get(c.Context(), actor.TeamID, actor.UserID)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", t)
}

func (h *TeamHandler) Update(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	var req updateTeamRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	t, err := h.svc.Update(c.Context(), actor.TeamID, actor.UserID, actor.Role, req.Name)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "team updated", t)
}

func (h *TeamHandler) ListMembers(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	members, err := h.svc.ListMembers(c.Context(), actor.TeamID, actor.UserID)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", fiber.Map{"members": members})
}

func (h *TeamHandler) AddMember(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	var req addMemberRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	member, err := h.svc.AddMember(c.Context(), actor.TeamID, actor.UserID, actor.Role, req.Email, team.Role(req.Role))
	if err != nil {
		return mapError(c, err)
	}
	return response.Created(c, "member added", member)
}

func (h *TeamHandler) ListUsers(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	users, err := h.svc.ListUsers(c.Context(), actor.TeamID, actor.UserID, actor.Role, team.ListMembersFilter{
		Search: c.Query("search"),
		Role:   c.Query("role"),
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", fiber.Map{"users": users})
}

func (h *TeamHandler) InviteUser(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	var req inviteUserRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	inv, err := h.svc.InviteUser(c.Context(), actor.TeamID, actor.UserID, actor.Role, req.Name, req.Email, team.Role(req.Role))
	if err != nil {
		return mapError(c, err)
	}
	return response.Created(c, "invitation sent", inv)
}

func (h *TeamHandler) UpdateUserRole(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	memberID, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid user id")
	}
	var req updateRoleRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	member, err := h.svc.UpdateMemberRole(c.Context(), actor.TeamID, actor.UserID, actor.Role, memberID, team.Role(req.Role))
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "role updated", member)
}

func (h *TeamHandler) DeactivateUser(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	memberID, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid user id")
	}
	member, err := h.svc.DeactivateMember(c.Context(), actor.TeamID, actor.UserID, actor.Role, memberID)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "user deactivated", member)
}

func (h *TeamHandler) RemoveUser(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	memberID, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid user id")
	}
	if err := h.svc.RemoveMember(c.Context(), actor.TeamID, actor.UserID, actor.Role, memberID); err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "user removed", nil)
}

func (h *TeamHandler) ListRoles(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	_ = actor
	if h.rbac == nil {
		return response.OK(c, "ok", fiber.Map{"roles": []any{}})
	}
	roles, err := h.rbac.ListRoles(c.Context())
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", fiber.Map{"roles": roles})
}
