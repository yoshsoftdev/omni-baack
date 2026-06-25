package handler

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/api/v1/dto"
	"github.com/omnichannel/omni-chan-v2/internal/campaign"
)

type CampaignHandler struct {
	svc *campaign.Service
}

func NewCampaignHandler(svc *campaign.Service) *CampaignHandler {
	return &CampaignHandler{svc: svc}
}

type createCampaignRequest struct {
	Name            string          `json:"name"`
	Description     *string         `json:"description"`
	ChannelID       string          `json:"channel_id"`
	DeviceID        string          `json:"device_id"`
	TemplateID      string          `json:"template_id"`
	SegmentID       *string         `json:"segment_id"`
	VariableMapping json.RawMessage `json:"variable_mapping"`
	SampleValues    json.RawMessage `json:"sample_values"`
	ContactIDs      []string        `json:"contact_ids"`
	TagIDs          []string        `json:"tag_ids"`
	ScheduledAt     *string         `json:"scheduled_at"`
	Timezone        *string         `json:"timezone"`
}

type updateCampaignRequest struct {
	Name            *string         `json:"name"`
	Description     *string         `json:"description"`
	VariableMapping json.RawMessage `json:"variable_mapping"`
	SampleValues    json.RawMessage `json:"sample_values"`
	ScheduledAt     *string         `json:"scheduled_at"`
	Timezone        *string         `json:"timezone"`
	SegmentID       *string         `json:"segment_id"`
}

func (h *CampaignHandler) List(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	var status *campaign.Status
	if raw := strings.TrimSpace(c.Query("status")); raw != "" {
		s := campaign.Status(raw)
		status = &s
	}
	channelID, err := optionalUUIDQuery(c, "channel_id")
	if err != nil {
		return response.BadRequest(c, "invalid channel_id")
	}
	params, err := pageParams(c)
	if err != nil {
		return mapError(c, err)
	}
	page, err := h.svc.List(c.Context(), actor.TeamID, actor.UserID, actor.Role, campaign.ListFilter{
		Search:    strings.TrimSpace(c.Query("search")),
		Status:    status,
		ChannelID: channelID,
	}, params)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", dto.NewPageResponse(page))
}

func (h *CampaignHandler) Get(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid campaign id")
	}
	camp, err := h.svc.Get(c.Context(), actor.TeamID, actor.UserID, actor.Role, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", camp)
}

func (h *CampaignHandler) Create(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	var req createCampaignRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	channelRaw := strings.TrimSpace(req.ChannelID)
	if channelRaw == "" {
		channelRaw = strings.TrimSpace(req.DeviceID)
	}
	channelID, err := uuid.Parse(channelRaw)
	if err != nil {
		return response.BadRequest(c, "invalid channel_id")
	}
	templateID, err := uuid.Parse(strings.TrimSpace(req.TemplateID))
	if err != nil {
		return response.BadRequest(c, "invalid template_id")
	}
	var segmentID *uuid.UUID
	if req.SegmentID != nil && strings.TrimSpace(*req.SegmentID) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(*req.SegmentID))
		if err != nil {
			return response.BadRequest(c, "invalid segment_id")
		}
		segmentID = &parsed
	}
	sendMode := campaign.SendNow
	var scheduledAt *time.Time
	if req.ScheduledAt != nil && strings.TrimSpace(*req.ScheduledAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.ScheduledAt))
		if err != nil {
			return response.BadRequest(c, "invalid scheduled_at")
		}
		scheduledAt = &parsed
		sendMode = campaign.SendLater
	}
	audiences := make([]campaign.AudienceInput, 0, len(req.ContactIDs)+len(req.TagIDs))
	for _, id := range parseUUIDList(req.ContactIDs) {
		audiences = append(audiences, campaign.AudienceInput{SourceType: campaign.AudienceContact, SourceID: id})
	}
	for _, id := range parseUUIDList(req.TagIDs) {
		audiences = append(audiences, campaign.AudienceInput{SourceType: campaign.AudienceTag, SourceID: id})
	}
	result, err := h.svc.Create(c.Context(), actor.TeamID, actor.UserID, actor.Role, campaign.CreateInput{
		Name:            strings.TrimSpace(req.Name),
		Description:     req.Description,
		ChannelID:       channelID,
		TemplateID:      templateID,
		SegmentID:       segmentID,
		Audiences:       audiences,
		VariableMapping: req.VariableMapping,
		SampleValues:    req.SampleValues,
		Timezone:        req.Timezone,
		SendMode:        sendMode,
		ScheduledAt:     scheduledAt,
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.Created(c, "campaign created", result)
}

func (h *CampaignHandler) Update(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid campaign id")
	}
	var req updateCampaignRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	var scheduledAt *time.Time
	var sendMode *campaign.SendMode
	if req.ScheduledAt != nil && strings.TrimSpace(*req.ScheduledAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.ScheduledAt))
		if err != nil {
			return response.BadRequest(c, "invalid scheduled_at")
		}
		scheduledAt = &parsed
		mode := campaign.SendLater
		sendMode = &mode
	}
	var segmentID *uuid.UUID
	if req.SegmentID != nil && strings.TrimSpace(*req.SegmentID) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(*req.SegmentID))
		if err != nil {
			return response.BadRequest(c, "invalid segment_id")
		}
		segmentID = &parsed
	}
	camp, err := h.svc.Update(c.Context(), actor.TeamID, actor.UserID, actor.Role, id, campaign.UpdateInput{
		Name:            req.Name,
		Description:     req.Description,
		VariableMapping: req.VariableMapping,
		SampleValues:    req.SampleValues,
		Timezone:        req.Timezone,
		SendMode:        sendMode,
		ScheduledAt:     scheduledAt,
		SegmentID:       segmentID,
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "campaign updated", camp)
}

func (h *CampaignHandler) Delete(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid campaign id")
	}
	if err := h.svc.Delete(c.Context(), actor.TeamID, actor.UserID, actor.Role, id); err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "campaign deleted", nil)
}

func (h *CampaignHandler) Send(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid campaign id")
	}
	result, err := h.svc.Send(c.Context(), actor.TeamID, actor.UserID, actor.Role, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "campaign queued for send", result)
}

type cancelCampaignRequest struct {
	Reason string `json:"reason"`
}

func (h *CampaignHandler) Cancel(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid campaign id")
	}
	var req cancelCampaignRequest
	_ = c.BodyParser(&req)
	camp, err := h.svc.Cancel(c.Context(), actor.TeamID, actor.UserID, actor.Role, id, strings.TrimSpace(req.Reason))
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "campaign cancelled", camp)
}

func parseUUIDList(values []string) []uuid.UUID {
	if len(values) == 0 {
		return nil
	}
	out := make([]uuid.UUID, 0, len(values))
	for _, raw := range values {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			continue
		}
		out = append(out, id)
	}
	return out
}
