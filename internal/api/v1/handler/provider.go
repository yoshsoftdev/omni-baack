package handler

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/provider"
)

type ProviderHandler struct {
	service *provider.Service
}

func NewProviderHandler(service *provider.Service) *ProviderHandler {
	return &ProviderHandler{service: service}
}

func (h *ProviderHandler) List(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	items, err := h.service.List(c.Context(), actor.TeamID, actor.UserID, actor.Role)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", fiber.Map{"providers": items})
}

func (h *ProviderHandler) Get(c *fiber.Ctx) error {
	actor, profileID, err := providerActorAndID(c)
	if err != nil {
		return response.BadRequest(c, "invalid provider id")
	}
	item, err := h.service.Get(c.Context(), actor.TeamID, actor.UserID, actor.Role, profileID)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", item)
}

func (h *ProviderHandler) ListByContact(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	contactID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid contact id")
	}
	items, err := h.service.ListByContact(c.Context(), actor.TeamID, actor.UserID, actor.Role, contactID)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", fiber.Map{"providers": items})
}

type linkProviderRequest struct {
	ContactID string `json:"contact_id"`
	Reason    string `json:"reason"`
}

func (h *ProviderHandler) Link(c *fiber.Ctx) error {
	actor, profileID, err := providerActorAndID(c)
	if err != nil {
		return response.BadRequest(c, "invalid provider id")
	}
	var req linkProviderRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	targetID, err := uuid.Parse(strings.TrimSpace(req.ContactID))
	if err != nil {
		return response.BadRequest(c, "invalid contact_id")
	}
	result, err := h.service.Link(c.Context(), actor.TeamID, actor.UserID, actor.Role, profileID, targetID, req.Reason)
	if err != nil {
		if errors.Is(err, provider.ErrCannotLinkToSameContact) {
			return response.BadRequest(c, err.Error())
		}
		return mapError(c, err)
	}
	return response.OK(c, "provider linked", result)
}

type unlinkProviderRequest struct {
	Reason string `json:"reason"`
}

func (h *ProviderHandler) Unlink(c *fiber.Ctx) error {
	actor, profileID, err := providerActorAndID(c)
	if err != nil {
		return response.BadRequest(c, "invalid provider id")
	}
	var req unlinkProviderRequest
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&req); err != nil {
			return response.BadRequest(c, "invalid request body")
		}
	}
	result, err := h.service.Unlink(c.Context(), actor.TeamID, actor.UserID, actor.Role, profileID, req.Reason)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "provider unlinked", result)
}

func (h *ProviderHandler) SetSourceOfTruth(c *fiber.Ctx) error {
	actor, profileID, err := providerActorAndID(c)
	if err != nil {
		return response.BadRequest(c, "invalid provider id")
	}
	item, err := h.service.SetSourceOfTruth(c.Context(), actor.TeamID, actor.UserID, actor.Role, profileID)
	if err != nil {
		if errors.Is(err, provider.ErrSourceOfTruthMustBeWhatsApp) {
			return response.BadRequest(c, err.Error())
		}
		return mapError(c, err)
	}
	return response.OK(c, "source of truth updated", item)
}

func (h *ProviderHandler) ListActivities(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	contactID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid contact id")
	}
	items, err := h.service.ListActivities(c.Context(), actor.TeamID, actor.UserID, actor.Role, contactID, c.QueryInt("limit", 100))
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", fiber.Map{"activities": items})
}

func providerActorAndID(c *fiber.Ctx) (actor struct {
	TeamID uuid.UUID
	UserID uuid.UUID
	Role   string
}, profileID uuid.UUID, err error) {
	resolved, err := requireActor(c)
	if err != nil {
		return actor, uuid.Nil, err
	}
	actor.TeamID = resolved.TeamID
	actor.UserID = resolved.UserID
	actor.Role = resolved.Role
	profileID, err = uuid.Parse(c.Params("id"))
	return actor, profileID, err
}
