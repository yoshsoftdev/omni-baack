package meta

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/internal/inbox"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
	"github.com/omnichannel/omni-chan-v2/pkg/logger"
)

type Handler struct {
	cfg    config.MetaConfig
	inbox  *inbox.Service
	log    *logger.Logger
}

func NewHandler(cfg config.MetaConfig, inboxService *inbox.Service, log *logger.Logger) *Handler {
	return &Handler{cfg: cfg, inbox: inboxService, log: log}
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

	messages, err := metaclient.ParseWebhookPayload(body)
	if err != nil {
		return response.BadRequest(c, "invalid webhook payload")
	}

	for _, msg := range messages {
		if err := h.inbox.HandleMetaInbound(c.Context(), msg); err != nil {
			h.log.Error("meta inbound failed", "error", err, "wamid", msg.WAMID)
		}
	}

	return c.SendStatus(fiber.StatusOK)
}
