package handler

import (
	"errors"
	"fmt"
	"strings"
	"time"

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
	baseURL := strings.TrimRight(strings.TrimSpace(c.Get("Origin")), "/")
	if baseURL == "" {
		baseURL = "http://localhost:3000"
	}
	expiresAt := inv.CreatedAt.Add(7 * 24 * time.Hour)
	return response.Created(c, "invitation created", fiber.Map{
		"invitation_id": inv.ID.String(),
		"invite_url":    fmt.Sprintf("%s/invite/%s", baseURL, inv.ID.String()),
		"expires_at":    expiresAt.UTC().Format(time.RFC3339),
		"role":          string(inv.Role),
	})
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

func (h *TeamHandler) GetRole(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	_ = actor
	if h.rbac == nil {
		return response.NotFound(c, "role not found")
	}
	detail, err := h.rbac.GetRole(c.Context(), c.Params("id"))
	if err != nil {
		return mapRbacError(c, err)
	}
	return response.OK(c, "ok", detail)
}

func (h *TeamHandler) GetPermissions(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	_ = actor
	if h.rbac == nil {
		return response.OK(c, "ok", fiber.Map{"matrix": []any{}})
	}
	matrix, err := h.rbac.GetPermissionMatrix(c.Context())
	if err != nil {
		return mapRbacError(c, err)
	}
	return response.OK(c, "ok", fiber.Map{"matrix": matrix})
}

type createRoleRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type updateRoleMetaRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type updateRolePermissionsRequest struct {
	Permissions []string `json:"permissions"`
}

func (h *TeamHandler) CreateRole(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	if h.rbac == nil {
		return response.InternalError(c, "rbac unavailable")
	}
	var req createRoleRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	role, err := h.rbac.CreateRole(c.Context(), actor.Role, req.Name, req.Description)
	if err != nil {
		return mapRbacError(c, err)
	}
	return response.Created(c, "role created", role)
}

func (h *TeamHandler) UpdateRole(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	if h.rbac == nil {
		return response.InternalError(c, "rbac unavailable")
	}
	var req updateRoleMetaRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	role, err := h.rbac.UpdateRoleMeta(c.Context(), actor.Role, c.Params("id"), req.Name, req.Description)
	if err != nil {
		return mapRbacError(c, err)
	}
	return response.OK(c, "role updated", role)
}

func (h *TeamHandler) DeleteRole(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	if h.rbac == nil {
		return response.InternalError(c, "rbac unavailable")
	}
	if err := h.rbac.DeleteRole(c.Context(), actor.Role, c.Params("id")); err != nil {
		return mapRbacError(c, err)
	}
	return response.OK(c, "role deleted", nil)
}

func (h *TeamHandler) UpdateRolePermissions(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	if h.rbac == nil {
		return response.InternalError(c, "rbac unavailable")
	}
	var req updateRolePermissionsRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if err := h.rbac.UpdateRolePermissions(c.Context(), actor.Role, c.Params("id"), req.Permissions); err != nil {
		return mapRbacError(c, err)
	}
	return response.OK(c, "role permissions updated", nil)
}

func mapRbacError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, rbac.ErrForbidden):
		return response.Forbidden(c, rbac.PermissionDeniedMessage)
	case errors.Is(err, rbac.ErrNotFound):
		return response.NotFound(c, err.Error())
	case errors.Is(err, rbac.ErrOwnerRoleImmutable), errors.Is(err, rbac.ErrSystemRoleDelete), errors.Is(err, rbac.ErrSystemRoleNameChange):
		return response.BadRequest(c, err.Error())
	default:
		if err.Error() == "name and description are required" || err.Error() == "description is required" || err.Error() == "name is required" || strings.Contains(err.Error(), "role name already exists") {
			return response.BadRequest(c, err.Error())
		}
		return mapError(c, err)
	}
}
