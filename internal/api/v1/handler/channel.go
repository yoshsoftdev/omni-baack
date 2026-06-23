package handler

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/api/v1/dto"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
)

type ChannelHandler struct {
	svc *channel.Service
}

func NewChannelHandler(svc *channel.Service) *ChannelHandler {
	return &ChannelHandler{svc: svc}
}

type createChannelRequest struct {
	Name           string `json:"name"`
	Provider       string `json:"provider"`
	ConnectionType string `json:"connection_type"`
	PhoneNumber    string `json:"phone_number"`
}

type updateChannelRequest struct {
	Name        *string `json:"name"`
	PhoneNumber *string `json:"phone_number"`
}

func (h *ChannelHandler) List(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	params, err := pageParams(c)
	if err != nil {
		return mapError(c, err)
	}
	page, err := h.svc.List(c.Context(), actor.TeamID, actor.UserID, actor.Role, channel.ListFilter{
		Search: strings.TrimSpace(c.Query("search")),
	}, params)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", dto.NewPageResponse(page))
}

func (h *ChannelHandler) Get(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid channel id")
	}
	ch, err := h.svc.Get(c.Context(), actor.TeamID, actor.UserID, actor.Role, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", ch)
}

func (h *ChannelHandler) Create(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	var req createChannelRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if req.Name == "" || req.Provider == "" {
		return response.BadRequest(c, "name and provider are required")
	}
	var connType *channel.ConnectionType
	if req.ConnectionType != "" {
		ct := channel.ConnectionType(req.ConnectionType)
		connType = &ct
	}
	ch, err := h.svc.Create(c.Context(), actor.TeamID, actor.UserID, actor.Role, channel.CreateInput{
		Name:           req.Name,
		Provider:       req.Provider,
		ConnectionType: connType,
		PhoneNumber:    optionalString(req.PhoneNumber),
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.Created(c, "channel created", ch)
}

func (h *ChannelHandler) Update(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid channel id")
	}
	var req updateChannelRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	ch, err := h.svc.Update(c.Context(), actor.TeamID, actor.UserID, actor.Role, id, channel.UpdateInput{
		Name:        req.Name,
		PhoneNumber: req.PhoneNumber,
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "channel updated", ch)
}

func (h *ChannelHandler) Delete(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid channel id")
	}
	if err := h.svc.Delete(c.Context(), actor.TeamID, actor.UserID, actor.Role, id); err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "channel deleted", nil)
}
