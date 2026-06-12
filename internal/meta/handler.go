package meta

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/internal/message"
	"github.com/omnichannel/omni-chan-v2/internal/onboarding"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

type Handler struct {
	cfg        config.MetaConfig
	messages   *message.Service
	onboarding *onboarding.Service
}

func NewHandler(cfg config.MetaConfig, messageService *message.Service, onboardingService *onboarding.Service) *Handler {
	return &Handler{cfg: cfg, messages: messageService, onboarding: onboardingService}
}

type EmbeddedConfigResponse struct {
	AppID      string `json:"app_id"`
	ConfigID   string `json:"config_id"`
	APIVersion string `json:"api_version"`
	Enabled    bool   `json:"enabled"`
}

// EmbeddedConfig returns public Meta Embedded Signup configuration for the frontend.
func (h *Handler) EmbeddedConfig(c *fiber.Ctx) error {
	enabled := h.cfg.AppID != "" && h.cfg.ConfigID != ""
	return response.OK(c, "ok", EmbeddedConfigResponse{
		AppID:      h.cfg.AppID,
		ConfigID:   h.cfg.ConfigID,
		APIVersion: h.cfg.APIVersion,
		Enabled:    enabled,
	})
}

// Verify handles Meta webhook subscription verification.
func (h *Handler) Verify(c *fiber.Ctx) error {
	mode := c.Query("hub.mode")
	token := c.Query("hub.verify_token")
	challenge := c.Query("hub.challenge")

	if mode == "subscribe" && token == h.cfg.VerifyToken {
		return c.SendString(challenge)
	}
	return response.Forbidden(c, "verification failed")
}

// Receive handles incoming WhatsApp webhook events from Meta.
func (h *Handler) Receive(c *fiber.Ctx) error {
	body := c.Body()
	if h.cfg.AppSecret != "" {
		sig := c.Get("X-Hub-Signature-256")
		if !metaclient.VerifySignature(h.cfg.AppSecret, sig, body) {
			return response.Forbidden(c, "invalid signature")
		}
	}

	events, err := metaclient.ParseCoexWebhookPayload(body)
	if err != nil {
		return response.BadRequest(c, "invalid webhook payload")
	}

	for _, msg := range events.InboundMessages {
		_ = h.messages.HandleMetaInbound(c.Context(), msg)
	}
	if h.onboarding != nil {
		_ = h.onboarding.HandleWebhook(c.Context(), events)
	}

	return c.SendStatus(fiber.StatusOK)
}
