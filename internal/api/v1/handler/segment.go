package handler

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/api/v1/dto"
	"github.com/omnichannel/omni-chan-v2/internal/segment"
)

type SegmentHandler struct {
	svc *segment.Service
}

func NewSegmentHandler(svc *segment.Service) *SegmentHandler {
	return &SegmentHandler{svc: svc}
}

type segmentRuleRequest struct {
	Field     string `json:"field"`
	Operator  string `json:"operator"`
	Value     string `json:"value"`
	SortOrder int    `json:"sort_order"`
}

type createSegmentRequest struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Color       string               `json:"color"`
	IsDynamic   bool                 `json:"is_dynamic"`
	Rules       []segmentRuleRequest `json:"rules"`
}

type updateSegmentRequest struct {
	Name        *string              `json:"name"`
	Description *string              `json:"description"`
	Color       *string              `json:"color"`
	IsDynamic   *bool                `json:"is_dynamic"`
	Rules       []segmentRuleRequest `json:"rules"`
}

func (h *SegmentHandler) List(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	params, err := pageParams(c)
	if err != nil {
		return mapError(c, err)
	}
	page, err := h.svc.List(c.Context(), actor.TeamID, actor.UserID, actor.Role, segment.ListFilter{
		Search: strings.TrimSpace(c.Query("search")),
	}, params)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", dto.NewPageResponse(page))
}

func (h *SegmentHandler) Get(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid segment id")
	}
	seg, rules, err := h.svc.Get(c.Context(), actor.TeamID, actor.UserID, actor.Role, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", fiber.Map{"segment": seg, "rules": rules})
}

func (h *SegmentHandler) Create(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	var req createSegmentRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	seg, rules, err := h.svc.Create(c.Context(), actor.TeamID, actor.UserID, actor.Role, segment.CreateInput{
		Name:        req.Name,
		Description: req.Description,
		Color:       req.Color,
		IsDynamic:   req.IsDynamic,
		Rules:       mapRules(req.Rules),
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.Created(c, "segment created", fiber.Map{"segment": seg, "rules": rules})
}

func (h *SegmentHandler) Update(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid segment id")
	}
	var req updateSegmentRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	var rulesInput []segment.RuleInput
	if len(req.Rules) > 0 {
		rulesInput = mapRules(req.Rules)
	}
	seg, rules, err := h.svc.Update(c.Context(), actor.TeamID, actor.UserID, actor.Role, id, segment.UpdateInput{
		Name:        req.Name,
		Description: req.Description,
		Color:       req.Color,
		IsDynamic:   req.IsDynamic,
		Rules:       rulesInput,
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "segment updated", fiber.Map{"segment": seg, "rules": rules})
}

func (h *SegmentHandler) Delete(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid segment id")
	}
	if err := h.svc.Delete(c.Context(), actor.TeamID, actor.UserID, actor.Role, id); err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "segment deleted", nil)
}

func mapRules(rules []segmentRuleRequest) []segment.RuleInput {
	if len(rules) == 0 {
		return nil
	}
	out := make([]segment.RuleInput, 0, len(rules))
	for _, rule := range rules {
		out = append(out, segment.RuleInput{
			Field:     segment.RuleField(rule.Field),
			Operator:  segment.RuleOperator(rule.Operator),
			Value:     rule.Value,
			SortOrder: rule.SortOrder,
		})
	}
	return out
}
