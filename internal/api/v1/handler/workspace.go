package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/workspace"
)

type WorkspaceHandler struct {
	svc *workspace.Service
}

func NewWorkspaceHandler(svc *workspace.Service) *WorkspaceHandler {
	return &WorkspaceHandler{svc: svc}
}

type updateWorkspaceRequest struct {
	Name     *string `json:"name"`
	Timezone *string `json:"timezone"`
	Country  *string `json:"country"`
}

type completeOnboardingStepRequest struct {
	Step int `json:"step"`
}

func (h *WorkspaceHandler) Get(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	team, err := h.svc.Get(c.Context(), actor.TeamID, actor.UserID, actor.Role)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", team)
}

func (h *WorkspaceHandler) Update(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	var req updateWorkspaceRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	team, err := h.svc.Update(c.Context(), actor.TeamID, actor.UserID, actor.Role, workspace.UpdateInput{
		Name:     req.Name,
		Timezone: req.Timezone,
		Country:  req.Country,
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "workspace updated", team)
}

func (h *WorkspaceHandler) GetQuotas(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	quotas, err := h.svc.GetQuotas(c.Context(), actor.TeamID, actor.UserID, actor.Role)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", quotas)
}

func (h *WorkspaceHandler) GetReadiness(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	readiness, err := h.svc.GetReadiness(c.Context(), actor.TeamID, actor.UserID, actor.Role)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", readiness)
}

func (h *WorkspaceHandler) GetOnboarding(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	snapshot, err := h.svc.GetOnboarding(c.Context(), actor.TeamID, actor.UserID, actor.Role)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", snapshot)
}

func (h *WorkspaceHandler) CompleteOnboardingStep(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	var req completeOnboardingStepRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if req.Step < 1 {
		return response.BadRequest(c, "step is required")
	}
	if err := h.svc.CompleteOnboardingStep(c.Context(), actor.TeamID, actor.UserID, actor.Role, req.Step); err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "step completed", nil)
}
