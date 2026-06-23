package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/api/v1/dto"
	"github.com/omnichannel/omni-chan-v2/internal/tag"
)

type TagHandler struct {
	svc *tag.Service
}

func NewTagHandler(svc *tag.Service) *TagHandler {
	return &TagHandler{svc: svc}
}

type createTagRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type updateTagRequest struct {
	Name  *string `json:"name"`
	Color *string `json:"color"`
}

func (h *TagHandler) List(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	params, err := pageParams(c)
	if err != nil {
		return mapError(c, err)
	}
	page, err := h.svc.List(c.Context(), actor.TeamID, actor.UserID, actor.Role, tag.ListFilter{
		Search: c.Query("search"),
	}, params)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", dto.NewPageResponse(page))
}

func (h *TagHandler) Get(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid tag id")
	}
	tg, err := h.svc.Get(c.Context(), actor.TeamID, actor.UserID, actor.Role, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", tg)
}

func (h *TagHandler) Create(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	var req createTagRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if req.Name == "" {
		return response.BadRequest(c, "name is required")
	}
	tg, err := h.svc.Create(c.Context(), actor.TeamID, actor.UserID, actor.Role, tag.CreateInput{
		Name:  req.Name,
		Color: req.Color,
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.Created(c, "tag created", tg)
}

func (h *TagHandler) Update(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid tag id")
	}
	var req updateTagRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	tg, err := h.svc.Update(c.Context(), actor.TeamID, actor.UserID, actor.Role, id, tag.UpdateInput{
		Name:  req.Name,
		Color: req.Color,
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "tag updated", tg)
}

func (h *TagHandler) Delete(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid tag id")
	}
	if err := h.svc.Delete(c.Context(), actor.TeamID, actor.UserID, actor.Role, id); err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "tag deleted", nil)
}

func (h *TagHandler) Assign(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	tagID, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid tag id")
	}
	contactID, err := parseUUIDParam(c, "contact_id")
	if err != nil {
		return response.BadRequest(c, "invalid contact id")
	}
	if err := h.svc.Assign(c.Context(), actor.TeamID, actor.UserID, actor.Role, tagID, contactID); err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "tag assigned", nil)
}

func (h *TagHandler) Unassign(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	tagID, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid tag id")
	}
	contactID, err := parseUUIDParam(c, "contact_id")
	if err != nil {
		return response.BadRequest(c, "invalid contact id")
	}
	if err := h.svc.Unassign(c.Context(), actor.TeamID, actor.UserID, actor.Role, tagID, contactID); err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "tag unassigned", nil)
}
