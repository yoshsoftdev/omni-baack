package team

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/rbac"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
)

type MgmtHandler struct {
	teamService *Service
	rbacService *rbac.Service
}

func NewMgmtHandler(teamService *Service, rbacService *rbac.Service) *MgmtHandler {
	return &MgmtHandler{teamService: teamService, rbacService: rbacService}
}

type inviteUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type updateRoleRequest struct {
	Role string `json:"role"`
}

func (h *MgmtHandler) ListUsers(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	users, err := h.teamService.ListUsers(c.Context(), claims.TeamID, claims.UserID, claims.Role, ListMembersFilter{
		Search: c.Query("search"),
		Role:   c.Query("role"),
	})
	if err != nil {
		return h.mapError(c, err)
	}

	return response.OK(c, "ok", fiber.Map{"users": users})
}

func (h *MgmtHandler) InviteUser(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	var req inviteUserRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if req.Email == "" || req.Role == "" {
		return response.BadRequest(c, "email and role are required")
	}

	inv, err := h.teamService.InviteUser(c.Context(), claims.TeamID, claims.UserID, claims.Role, req.Name, req.Email, Role(req.Role))
	if err != nil {
		return h.mapError(c, err)
	}

	return response.Created(c, "invitation sent", fiber.Map{
		"id":         inv.ID,
		"email":      inv.Email,
		"name":       inv.Name,
		"role":       inv.Role,
		"status":     inv.Status,
		"created_at": inv.CreatedAt,
	})
}

func (h *MgmtHandler) UpdateUserRole(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	memberID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid user id")
	}

	var req updateRoleRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if req.Role == "" {
		return response.BadRequest(c, "role is required")
	}

	member, err := h.teamService.UpdateMemberRole(c.Context(), claims.TeamID, claims.UserID, claims.Role, memberID, Role(req.Role))
	if err != nil {
		return h.mapError(c, err)
	}

	return response.OK(c, "role updated", member)
}

func (h *MgmtHandler) DeactivateUser(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	memberID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid user id")
	}

	member, err := h.teamService.DeactivateMember(c.Context(), claims.TeamID, claims.UserID, claims.Role, memberID)
	if err != nil {
		return h.mapError(c, err)
	}

	return response.OK(c, "user deactivated", member)
}

func (h *MgmtHandler) RemoveUser(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	memberID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid user id")
	}

	if err := h.teamService.RemoveMember(c.Context(), claims.TeamID, claims.UserID, claims.Role, memberID); err != nil {
		return h.mapError(c, err)
	}

	return response.OK(c, "user removed", nil)
}

func (h *MgmtHandler) ListRoles(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	if !h.rbacService.Require(claims.Role, rbac.PermUserView) {
		return response.Forbidden(c, rbac.PermissionDeniedMessage)
	}

	roles, err := h.rbacService.ListRoles(c.Context())
	if err != nil {
		return response.InternalError(c, "something went wrong")
	}

	return response.OK(c, "ok", fiber.Map{"roles": roles})
}

func (h *MgmtHandler) GetRole(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	if !h.rbacService.Require(claims.Role, rbac.PermUserView) {
		return response.Forbidden(c, rbac.PermissionDeniedMessage)
	}

	detail, err := h.rbacService.GetRole(c.Context(), c.Params("id"))
	if err != nil {
		if errors.Is(err, rbac.ErrNotFound) {
			return response.NotFound(c, err.Error())
		}
		return response.InternalError(c, "something went wrong")
	}

	return response.OK(c, "ok", detail)
}

func (h *MgmtHandler) GetPermissions(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	if !h.rbacService.Require(claims.Role, rbac.PermTeamManage) {
		return response.Forbidden(c, rbac.PermissionDeniedMessage)
	}

	matrix, err := h.rbacService.GetPermissionMatrix(c.Context())
	if err != nil {
		return response.InternalError(c, "something went wrong")
	}

	return response.OK(c, "ok", fiber.Map{"matrix": matrix})
}

func (h *MgmtHandler) GetWorkspace(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	workspace, err := h.teamService.GetWorkspace(c.Context(), claims.TeamID, claims.UserID, claims.Role)
	if err != nil {
		return h.mapError(c, err)
	}

	return response.OK(c, "ok", workspace)
}

func (h *MgmtHandler) mapError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrForbidden):
		return response.Forbidden(c, rbac.PermissionDeniedMessage)
	case errors.Is(err, ErrInvalidRole), errors.Is(err, ErrAlreadyMember), errors.Is(err, ErrCannotModifyOwner):
		return response.BadRequest(c, err.Error())
	case errors.Is(err, ErrUserNotFound), errors.Is(err, ErrMemberNotFound), errors.Is(err, ErrNotFound):
		return response.NotFound(c, err.Error())
	default:
		if err.Error() == "email is required" || err.Error() == "team name is required" {
			return response.BadRequest(c, err.Error())
		}
		return response.InternalError(c, "something went wrong")
	}
}

func RegisterMgmtRoutes(router fiber.Router, handler *MgmtHandler, jwt *jwtmanager.Manager) {
	teamGroup := router.Group("/team", middleware.Auth(jwt))

	teamGroup.Get("/users", rbac.RequirePermission(handler.rbacService, rbac.PermUserView), handler.ListUsers)
	teamGroup.Post("/users/invite", rbac.RequirePermission(handler.rbacService, rbac.PermUserInvite), handler.InviteUser)
	teamGroup.Put("/users/:id/role", rbac.RequirePermission(handler.rbacService, rbac.PermUserUpdate), handler.UpdateUserRole)
	teamGroup.Patch("/users/:id/deactivate", rbac.RequirePermission(handler.rbacService, rbac.PermUserUpdate), handler.DeactivateUser)
	teamGroup.Delete("/users/:id", rbac.RequirePermission(handler.rbacService, rbac.PermUserDelete), handler.RemoveUser)

	teamGroup.Get("/roles", handler.ListRoles)
	teamGroup.Get("/roles/:id", handler.GetRole)
	teamGroup.Get("/permissions", handler.GetPermissions)
	teamGroup.Get("/workspace", handler.GetWorkspace)
}
