package handler

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/api/v1/dto"
	"github.com/omnichannel/omni-chan-v2/internal/channel"
	"github.com/omnichannel/omni-chan-v2/internal/channelsync"
	metasvc "github.com/omnichannel/omni-chan-v2/internal/meta"
)

type ChannelHandler struct {
	svc            *channel.Service
	health         *metasvc.HealthService
	embeddedSignup *metasvc.EmbeddedSignupService
	payment        *metasvc.PaymentService
	sync           *channelsync.Service
}

func NewChannelHandler(
	svc *channel.Service,
	health *metasvc.HealthService,
	embeddedSignup *metasvc.EmbeddedSignupService,
	payment *metasvc.PaymentService,
	syncSvc *channelsync.Service,
) *ChannelHandler {
	return &ChannelHandler{svc: svc, health: health, embeddedSignup: embeddedSignup, payment: payment, sync: syncSvc}
}

type createChannelRequest struct {
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	ConnectionType  string `json:"connection_type"`
	PhoneNumber     string `json:"phone_number"`
	AccessToken     string `json:"access_token"`
	WABAID          string `json:"waba_id"`
	PhoneNumberID   string `json:"phone_number_id"`
	BusinessID      string `json:"business_id"`
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

type verifyManualConnectRequest struct {
	AccessToken   string `json:"access_token"`
	WABAID        string `json:"waba_id"`
	PhoneNumberID string `json:"phone_number_id"`
}

func (h *ChannelHandler) VerifyManualConnect(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	var req verifyManualConnectRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	result, err := h.svc.VerifyManualConnect(c.Context(), actor.TeamID, actor.UserID, actor.Role, channel.ManualConnectInput{
		AccessToken:   req.AccessToken,
		WABAID:        req.WABAID,
		PhoneNumberID: req.PhoneNumberID,
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "manual connection verified", result)
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
	var manual *channel.ManualConnectInput
	if strings.TrimSpace(req.AccessToken) != "" || strings.TrimSpace(req.WABAID) != "" || strings.TrimSpace(req.PhoneNumberID) != "" {
		manual = &channel.ManualConnectInput{
			AccessToken:   req.AccessToken,
			WABAID:        req.WABAID,
			PhoneNumberID: req.PhoneNumberID,
			BusinessID:    optionalString(req.BusinessID),
		}
		if connType == nil {
			ct := channel.ConnectionManual
			connType = &ct
		}
	}
	ch, err := h.svc.Create(c.Context(), actor.TeamID, actor.UserID, actor.Role, channel.CreateInput{
		Name:           req.Name,
		Provider:       req.Provider,
		ConnectionType: connType,
		PhoneNumber:    optionalString(req.PhoneNumber),
		BusinessID:     optionalString(req.BusinessID),
		Manual:         manual,
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

func (h *ChannelHandler) Health(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid channel id")
	}
	if h.health == nil {
		return response.FeatureNotAvailable(c, "channel health checks are not configured")
	}
	result, err := h.health.CheckChannel(c.Context(), actor.TeamID, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", result)
}

func (h *ChannelHandler) StartTeamEmbeddedSignup(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	if h.embeddedSignup == nil {
		return response.FeatureNotAvailable(c, "embedded signup is not configured")
	}
	result, err := h.embeddedSignup.StartTeamEmbeddedSignup(c.Context(), actor.TeamID, actor.UserID)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "embedded signup started", result)
}

func (h *ChannelHandler) CompleteTeamEmbeddedSignup(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	sessionID := c.Params("session_id")
	if sessionID == "" {
		return response.BadRequest(c, "session_id is required")
	}
	var req completeEmbeddedSignupRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if h.embeddedSignup == nil {
		return response.FeatureNotAvailable(c, "embedded signup is not configured")
	}
	result, err := h.embeddedSignup.CompleteTeamEmbeddedSignup(c.Context(), actor.TeamID, actor.UserID, sessionID, metasvc.CompleteEmbeddedSignupInput{
		Code:          req.Code,
		OAuthState:    req.OAuthState,
		BusinessID:    req.BusinessID,
		WABAID:        req.WABAID,
		PhoneNumberID: req.PhoneNumberID,
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "embedded signup completed", result)
}

func (h *ChannelHandler) GetTeamEmbeddedSignupStatus(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	sessionID := c.Params("session_id")
	if sessionID == "" {
		return response.BadRequest(c, "session_id is required")
	}
	if h.embeddedSignup == nil {
		return response.FeatureNotAvailable(c, "embedded signup is not configured")
	}
	result, err := h.embeddedSignup.GetTeamEmbeddedSignupStatus(c.Context(), actor.TeamID, sessionID)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", result)
}

func (h *ChannelHandler) StartEmbeddedSignup(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid channel id")
	}
	if h.embeddedSignup == nil {
		return response.FeatureNotAvailable(c, "embedded signup is not configured")
	}
	result, err := h.embeddedSignup.StartEmbeddedSignup(c.Context(), actor.TeamID, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "embedded signup started", result)
}

type completeEmbeddedSignupRequest struct {
	Code          string `json:"code"`
	OAuthState    string `json:"oauth_state"`
	BusinessID    string `json:"business_id"`
	WABAID        string `json:"waba_id"`
	PhoneNumberID string `json:"phone_number_id"`
}

func (h *ChannelHandler) CompleteEmbeddedSignup(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	channelID, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid channel id")
	}
	sessionID := c.Params("session_id")
	if sessionID == "" {
		return response.BadRequest(c, "session_id is required")
	}
	var req completeEmbeddedSignupRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if h.embeddedSignup == nil {
		return response.FeatureNotAvailable(c, "embedded signup is not configured")
	}
	result, err := h.embeddedSignup.CompleteEmbeddedSignup(c.Context(), actor.TeamID, channelID, sessionID, metasvc.CompleteEmbeddedSignupInput{
		Code:          req.Code,
		OAuthState:    req.OAuthState,
		BusinessID:    req.BusinessID,
		WABAID:        req.WABAID,
		PhoneNumberID: req.PhoneNumberID,
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "embedded signup completed", result)
}

func (h *ChannelHandler) GetEmbeddedSignupStatus(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	channelID, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid channel id")
	}
	sessionID := c.Params("session_id")
	if sessionID == "" {
		return response.BadRequest(c, "session_id is required")
	}
	if h.embeddedSignup == nil {
		return response.FeatureNotAvailable(c, "embedded signup is not configured")
	}
	result, err := h.embeddedSignup.GetEmbeddedSignupStatus(c.Context(), actor.TeamID, channelID, sessionID)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", result)
}

func (h *ChannelHandler) GetPaymentStatus(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid channel id")
	}
	if h.payment == nil {
		return response.FeatureNotAvailable(c, "payment status checks are not configured")
	}
	result, err := h.payment.GetPaymentStatus(c.Context(), actor.TeamID, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", result)
}

func (h *ChannelHandler) CreatePaymentLink(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid channel id")
	}
	if h.payment == nil {
		return response.FeatureNotAvailable(c, "payment setup is not configured")
	}
	result, err := h.payment.GetPaymentLink(c.Context(), actor.TeamID, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", result)
}

func (h *ChannelHandler) GetSyncStatus(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid channel id")
	}
	if h.sync == nil {
		return response.FeatureNotAvailable(c, "channel synchronization is not configured")
	}
	result, err := h.sync.GetStatus(c.Context(), actor.TeamID, actor.UserID, actor.Role, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", result)
}

func (h *ChannelHandler) SyncContacts(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid channel id")
	}
	if h.sync == nil {
		return response.FeatureNotAvailable(c, "channel synchronization is not configured")
	}
	result, err := h.sync.SyncContacts(c.Context(), actor.TeamID, actor.UserID, actor.Role, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "contacts synchronized", result)
}

func (h *ChannelHandler) SyncConversations(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid channel id")
	}
	if h.sync == nil {
		return response.FeatureNotAvailable(c, "channel synchronization is not configured")
	}
	result, err := h.sync.SyncConversations(c.Context(), actor.TeamID, actor.UserID, actor.Role, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "conversation synchronization completed", result)
}

func (h *ChannelHandler) GetPermissions(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid channel id")
	}
	if h.sync == nil {
		return response.FeatureNotAvailable(c, "channel synchronization is not configured")
	}
	result, err := h.sync.GetPermissions(c.Context(), actor.TeamID, actor.UserID, actor.Role, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", result)
}

func (h *ChannelHandler) ReauthorizePermissions(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid channel id")
	}
	if h.embeddedSignup == nil {
		return response.FeatureNotAvailable(c, "embedded signup is not configured")
	}
	result, err := h.embeddedSignup.StartEmbeddedSignup(c.Context(), actor.TeamID, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "meta reauthorization started", result)
}
