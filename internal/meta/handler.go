package meta

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/internal/message"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

type Handler struct {
	cfg      config.MetaConfig
	messages *message.Service
}

func NewHandler(cfg config.MetaConfig, messageService *message.Service) *Handler {
	return &Handler{cfg: cfg, messages: messageService}
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

	inboundMessages, err := metaclient.ParseWebhookPayload(body)
	if err != nil {
		return response.BadRequest(c, "invalid webhook payload")
	}

	for _, msg := range inboundMessages {
		_ = h.messages.HandleMetaInbound(c.Context(), msg)
	}

	return c.SendStatus(fiber.StatusOK)
}
