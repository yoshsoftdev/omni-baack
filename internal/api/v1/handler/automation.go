package handler

import (
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/api/v1/dto"
	"github.com/omnichannel/omni-chan-v2/internal/automation"
)

type AutomationHandler struct {
	svc           *automation.Service
	internalToken string
}

func NewAutomationHandler(svc *automation.Service, internalToken string) *AutomationHandler {
	return &AutomationHandler{
		svc:           svc,
		internalToken: strings.TrimSpace(internalToken),
	}
}

type createEndpointRequest struct {
	Name         string          `json:"name"`
	Slug         string          `json:"slug"`
	Description  string          `json:"description"`
	ActionType   string          `json:"actionType"`
	ActionConfig json.RawMessage `json:"actionConfig"`
	Webhook      *webhookConfig  `json:"webhook"`
}

type updateEndpointRequest struct {
	Name         *string         `json:"name"`
	Slug         *string         `json:"slug"`
	Description  *string         `json:"description"`
	ActionConfig json.RawMessage `json:"actionConfig"`
	Status       *string         `json:"status"`
	Webhook      *webhookConfig  `json:"webhook"`
}

type webhookConfig struct {
	URL       string `json:"url"`
	IsEnabled bool   `json:"isEnabled"`
	Secret    string `json:"secret"`
}

type createAPIKeyRequest struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
}

type createForwardWebhookRequest struct {
	Name                     string   `json:"name"`
	URL                      string   `json:"url"`
	Secret                   string   `json:"secret"`
	Events                   []string `json:"events"`
	ChannelID                *string  `json:"channelId"`
	OnlyUnassigned           bool     `json:"onlyUnassigned"`
	AssignAutomationOnReopen bool     `json:"assignAutomationOnReopen"`
}

type updateForwardWebhookRequest struct {
	Name                     *string  `json:"name"`
	URL                      *string  `json:"url"`
	Status                   *string  `json:"status"`
	Events                   []string `json:"events"`
	ChannelID                *string  `json:"channelId"`
	OnlyUnassigned           *bool    `json:"onlyUnassigned"`
	AssignAutomationOnReopen *bool    `json:"assignAutomationOnReopen"`
	ClearChannel             bool     `json:"clearChannel"`
	RotateSecret             bool     `json:"rotateSecret"`
	Secret                   *string  `json:"secret"`
}

type updateInboxAutomationSettingsRequest struct {
	Enabled bool `json:"enabled"`
}

func (h *AutomationHandler) Summary(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	summary, err := h.svc.GetSummary(c.Context(), actor.TeamID, actor.UserID, actor.Role)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", summary)
}

func (h *AutomationHandler) ListEndpoints(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	params, err := pageParams(c)
	if err != nil {
		return mapError(c, err)
	}
	page, err := h.svc.ListEndpoints(c.Context(), actor.TeamID, actor.UserID, actor.Role, automation.EndpointListFilter{
		Search:     c.Query("search"),
		Status:     c.Query("status", "all"),
		ActionType: c.Query("actionType", "all"),
	}, params)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", dto.NewPageResponse(page))
}

func (h *AutomationHandler) GetEndpoint(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id := strings.TrimSpace(c.Params("id"))
	item, err := h.svc.GetEndpoint(c.Context(), actor.TeamID, actor.UserID, actor.Role, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", item)
}

func (h *AutomationHandler) CreateEndpoint(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	var req createEndpointRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	item, err := h.svc.CreateEndpoint(c.Context(), actor.TeamID, actor.UserID, actor.Role, automation.CreateEndpointInput{
		Name:         req.Name,
		Slug:         req.Slug,
		Description:  req.Description,
		ActionType:   automation.ActionType(req.ActionType),
		ActionConfig: req.ActionConfig,
		Webhook:      toDomainWebhook(req.Webhook),
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.Created(c, "endpoint created", item)
}

func (h *AutomationHandler) UpdateEndpoint(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id := strings.TrimSpace(c.Params("id"))
	var req updateEndpointRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	input := automation.UpdateEndpointInput{
		Name:         req.Name,
		Slug:         req.Slug,
		Description:  req.Description,
		ActionConfig: req.ActionConfig,
		Webhook:      toDomainWebhook(req.Webhook),
	}
	if req.Status != nil {
		status := automation.EndpointStatus(*req.Status)
		input.Status = &status
	}
	item, err := h.svc.UpdateEndpoint(c.Context(), actor.TeamID, actor.UserID, actor.Role, id, input)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "endpoint updated", item)
}

func toDomainWebhook(in *webhookConfig) *automation.WebhookConfig {
	if in == nil {
		return nil
	}
	return &automation.WebhookConfig{
		URL:       in.URL,
		IsEnabled: in.IsEnabled,
		Secret:    in.Secret,
	}
}

func (h *AutomationHandler) DisableEndpoint(c *fiber.Ctx) error {
	return h.setEndpointStatus(c, automation.EndpointStatusDisabled)
}

func (h *AutomationHandler) EnableEndpoint(c *fiber.Ctx) error {
	return h.setEndpointStatus(c, automation.EndpointStatusActive)
}

func (h *AutomationHandler) setEndpointStatus(c *fiber.Ctx, status automation.EndpointStatus) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id := strings.TrimSpace(c.Params("id"))
	item, err := h.svc.SetEndpointStatus(c.Context(), actor.TeamID, actor.UserID, actor.Role, id, status)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "endpoint updated", item)
}

func (h *AutomationHandler) DuplicateEndpoint(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id := strings.TrimSpace(c.Params("id"))
	item, err := h.svc.DuplicateEndpoint(c.Context(), actor.TeamID, actor.UserID, actor.Role, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.Created(c, "endpoint duplicated", item)
}

func (h *AutomationHandler) DeleteEndpoint(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id := strings.TrimSpace(c.Params("id"))
	if err := h.svc.DeleteEndpoint(c.Context(), actor.TeamID, actor.UserID, actor.Role, id); err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "endpoint deleted", nil)
}

func (h *AutomationHandler) Analytics(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id := strings.TrimSpace(c.Params("id"))
	item, err := h.svc.GetAnalytics(c.Context(), actor.TeamID, actor.UserID, actor.Role, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", item)
}

func (h *AutomationHandler) TestInvoke(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id := strings.TrimSpace(c.Params("id"))
	var body map[string]any
	if err := c.BodyParser(&body); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	result, err := h.svc.TestInvoke(c.Context(), actor.TeamID, actor.UserID, actor.Role, id, body, c.IP())
	if err != nil {
		return mapError(c, err)
	}
	return c.Status(result.StatusCode).JSON(result.Body)
}

func (h *AutomationHandler) ListAPIKeys(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	items, err := h.svc.ListAPIKeys(c.Context(), actor.TeamID, actor.UserID, actor.Role)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", items)
}

func (h *AutomationHandler) CreateAPIKey(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	var req createAPIKeyRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	result, err := h.svc.CreateAPIKey(c.Context(), actor.TeamID, actor.UserID, actor.Role, automation.CreateAPIKeyInput{
		Name:   req.Name,
		Scopes: req.Scopes,
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.Created(c, "api key created", result)
}

func (h *AutomationHandler) RotateAPIKey(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id := strings.TrimSpace(c.Params("id"))
	result, err := h.svc.RotateAPIKey(c.Context(), actor.TeamID, actor.UserID, actor.Role, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "api key rotated", result)
}

func (h *AutomationHandler) DisableAPIKey(c *fiber.Ctx) error {
	return h.setAPIKeyStatus(c, automation.APIKeyStatusDisabled)
}

func (h *AutomationHandler) EnableAPIKey(c *fiber.Ctx) error {
	return h.setAPIKeyStatus(c, automation.APIKeyStatusActive)
}

func (h *AutomationHandler) setAPIKeyStatus(c *fiber.Ctx, status automation.APIKeyStatus) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id := strings.TrimSpace(c.Params("id"))
	item, err := h.svc.SetAPIKeyStatus(c.Context(), actor.TeamID, actor.UserID, actor.Role, id, status)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "api key updated", item)
}

func (h *AutomationHandler) DeleteAPIKey(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id := strings.TrimSpace(c.Params("id"))
	if err := h.svc.DeleteAPIKey(c.Context(), actor.TeamID, actor.UserID, actor.Role, id); err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "api key deleted", nil)
}

func (h *AutomationHandler) ListForwardWebhooks(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	items, err := h.svc.ListForwardWebhooks(c.Context(), actor.TeamID, actor.UserID, actor.Role)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", items)
}

func (h *AutomationHandler) GetInboxAutomationSettings(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	settings, err := h.svc.GetInboxAutomationSettings(c.Context(), actor.TeamID, actor.UserID, actor.Role)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", settings)
}

func (h *AutomationHandler) UpdateInboxAutomationSettings(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	var req updateInboxAutomationSettingsRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	settings, err := h.svc.SetInboxAutomationSettings(c.Context(), actor.TeamID, actor.UserID, actor.Role, req.Enabled)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "automation inbox setting updated", settings)
}

func (h *AutomationHandler) CreateForwardWebhook(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	var req createForwardWebhookRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	var channelID *uuid.UUID
	if req.ChannelID != nil && strings.TrimSpace(*req.ChannelID) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(*req.ChannelID))
		if err != nil {
			return response.BadRequest(c, "invalid channelId")
		}
		channelID = &parsed
	}
	item, err := h.svc.CreateForwardWebhook(c.Context(), actor.TeamID, actor.UserID, actor.Role, automation.CreateForwardWebhookInput{
		Name:                     req.Name,
		URL:                      req.URL,
		Secret:                   req.Secret,
		Events:                   req.Events,
		ChannelID:                channelID,
		OnlyUnassigned:           req.OnlyUnassigned,
		AssignAutomationOnReopen: req.AssignAutomationOnReopen,
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.Created(c, "webhook created", item)
}

func (h *AutomationHandler) UpdateForwardWebhook(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id := strings.TrimSpace(c.Params("id"))
	var req updateForwardWebhookRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	input := automation.UpdateForwardWebhookInput{
		Name:                     req.Name,
		URL:                      req.URL,
		Events:                   req.Events,
		OnlyUnassigned:           req.OnlyUnassigned,
		AssignAutomationOnReopen: req.AssignAutomationOnReopen,
		ClearChannel:             req.ClearChannel,
		RotateSecret:             req.RotateSecret,
		Secret:                   req.Secret,
	}
	if req.Status != nil {
		status := automation.ForwardWebhookStatus(*req.Status)
		input.Status = &status
	}
	if req.ChannelID != nil && strings.TrimSpace(*req.ChannelID) != "" {
		parsed, err := uuid.Parse(strings.TrimSpace(*req.ChannelID))
		if err != nil {
			return response.BadRequest(c, "invalid channelId")
		}
		input.ChannelID = &parsed
	}
	item, err := h.svc.UpdateForwardWebhook(c.Context(), actor.TeamID, actor.UserID, actor.Role, id, input)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "webhook updated", item)
}

func (h *AutomationHandler) EnableForwardWebhook(c *fiber.Ctx) error {
	return h.setForwardWebhookStatus(c, automation.ForwardWebhookActive)
}

func (h *AutomationHandler) DisableForwardWebhook(c *fiber.Ctx) error {
	return h.setForwardWebhookStatus(c, automation.ForwardWebhookDisabled)
}

func (h *AutomationHandler) setForwardWebhookStatus(c *fiber.Ctx, status automation.ForwardWebhookStatus) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id := strings.TrimSpace(c.Params("id"))
	item, err := h.svc.SetForwardWebhookStatus(c.Context(), actor.TeamID, actor.UserID, actor.Role, id, status)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "webhook updated", item)
}

func (h *AutomationHandler) DeleteForwardWebhook(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id := strings.TrimSpace(c.Params("id"))
	if err := h.svc.DeleteForwardWebhook(c.Context(), actor.TeamID, actor.UserID, actor.Role, id); err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "webhook deleted", nil)
}

func (h *AutomationHandler) ListForwardDeliveries(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	params, err := pageParams(c)
	if err != nil {
		return mapError(c, err)
	}
	id := strings.TrimSpace(c.Params("id"))
	page, err := h.svc.ListForwardDeliveries(c.Context(), actor.TeamID, actor.UserID, actor.Role, id, params)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", dto.NewPageResponse(page))
}

// InvokeInternal handles POST /internal/automation/endpoints/:id.
// Only webhook-service may call this (X-Internal-Token). Integrator auth is Bearer oc_auto_*.
func (h *AutomationHandler) InvokeInternal(c *fiber.Ctx) error {
	if h.svc == nil || h.svc.Invoker() == nil {
		return response.InternalError(c, "automation invoke is not configured")
	}
	if h.internalToken == "" || strings.TrimSpace(c.Get("X-Internal-Token")) != h.internalToken {
		return response.Unauthorized(c, "unauthorized")
	}
	id := strings.TrimSpace(c.Params("id"))
	var body map[string]any
	if err := c.BodyParser(&body); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	clientIP := strings.TrimSpace(c.Get("X-Forwarded-For"))
	if clientIP == "" {
		clientIP = c.IP()
	} else {
		clientIP = strings.TrimSpace(strings.Split(clientIP, ",")[0])
	}
	authHeader := c.Get("Authorization")
	result, err := h.svc.Invoker().InvokePublic(c.Context(), id, authHeader, body, clientIP)
	if err != nil {
		return mapError(c, err)
	}
	return c.Status(result.StatusCode).JSON(result.Body)
}
