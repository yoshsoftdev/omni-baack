package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/api/v1/dto"
	"github.com/omnichannel/omni-chan-v2/internal/quickreply"
)

type QuickReplyHandler struct {
	svc *quickreply.Service
}

func NewQuickReplyHandler(svc *quickreply.Service) *QuickReplyHandler {
	return &QuickReplyHandler{svc: svc}
}

type createQuickReplyRequest struct {
	Category string `json:"category"`
	Content  string `json:"content"`
	Text     string `json:"text"`
}

type updateQuickReplyRequest struct {
	Category *string `json:"category"`
	Content  *string `json:"content"`
	Text     *string `json:"text"`
}

func quickReplyContent(req createQuickReplyRequest) string {
	if req.Content != "" {
		return req.Content
	}
	return req.Text
}

func quickReplyUpdateContent(req updateQuickReplyRequest) *string {
	if req.Content != nil {
		return req.Content
	}
	return req.Text
}

func (h *QuickReplyHandler) List(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	params, err := pageParams(c)
	if err != nil {
		return mapError(c, err)
	}
	page, err := h.svc.List(c.Context(), actor.TeamID, actor.UserID, actor.Role, quickreply.ListFilter{
		Search: c.Query("search"),
	}, params)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", dto.NewPageResponse(page))
}

func (h *QuickReplyHandler) Get(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid quick reply id")
	}
	item, err := h.svc.Get(c.Context(), actor.TeamID, actor.UserID, actor.Role, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", item)
}

func (h *QuickReplyHandler) Create(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	var req createQuickReplyRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	content := quickReplyContent(req)
	if content == "" {
		return response.BadRequest(c, "content is required")
	}
	item, err := h.svc.Create(c.Context(), actor.TeamID, actor.UserID, actor.Role, quickreply.CreateInput{
		Category: req.Category,
		Content:  content,
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.Created(c, "quick reply created", item)
}

func (h *QuickReplyHandler) Update(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid quick reply id")
	}
	var req updateQuickReplyRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	item, err := h.svc.Update(c.Context(), actor.TeamID, actor.UserID, actor.Role, id, quickreply.UpdateInput{
		Category: req.Category,
		Content:  quickReplyUpdateContent(req),
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "quick reply updated", item)
}

func (h *QuickReplyHandler) Delete(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid quick reply id")
	}
	if err := h.svc.Delete(c.Context(), actor.TeamID, actor.UserID, actor.Role, id); err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "quick reply deleted", nil)
}
