package meta

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/config"
	"github.com/omnichannel/omni-chan-v2/internal/message"
	metaclient "github.com/omnichannel/omni-chan-v2/pkg/meta"
	"github.com/omnichannel/omni-chan-v2/pkg/logger"
)

type Handler struct {
	cfg      config.MetaConfig
	messages *message.Service
	log      *logger.Logger
}

func NewHandler(cfg config.MetaConfig, messageService *message.Service, log *logger.Logger) *Handler {
	return &Handler{cfg: cfg, messages: messageService, log: log}
}

// Verify handles Meta webhook subscription verification.
func (h *Handler) Verify(c *fiber.Ctx) error {
	mode := c.Query("hub.mode")
	token := c.Query("hub.verify_token")
	challenge := c.Query("hub.challenge")

	h.log.Info("meta webhook verify",
		"mode", mode,
		"token_match", token == h.cfg.VerifyToken,
		"has_challenge", challenge != "",
		"remote_ip", c.IP(),
	)

	if mode == "subscribe" && token == h.cfg.VerifyToken {
		h.log.Info("meta webhook verify success")
		return c.SendString(challenge)
	}

	h.log.Warn("meta webhook verify failed",
		"expected_token_set", h.cfg.VerifyToken != "",
	)
	return response.Forbidden(c, "verification failed")
}

// Receive handles incoming WhatsApp webhook events from Meta.
func (h *Handler) Receive(c *fiber.Ctx) error {
	body := c.Body()
	signature := c.Get("X-Hub-Signature-256")

	h.log.Info("meta webhook received",
		"method", c.Method(),
		"path", c.Path(),
		"remote_ip", c.IP(),
		"content_type", c.Get("Content-Type"),
		"body_bytes", len(body),
		"has_signature", metaclient.HasSignatureHeader(signature),
		"app_secret_configured", h.cfg.AppSecret != "",
	)

	if len(body) > 0 {
		h.log.Debug("meta webhook raw body",
			"body", metaclient.TruncateForLog(string(body), 4096),
		)
	}

	summary, err := metaclient.SummarizeWebhookPayload(body)
	if err != nil {
		h.log.Error("meta webhook summarize failed", "error", err)
		return response.BadRequest(c, "invalid webhook payload")
	}

	h.log.Info("meta webhook payload summary",
		"object", summary.Object,
		"entry_count", summary.EntryCount,
		"change_fields", summary.ChangeFields,
		"phone_number_ids", summary.PhoneNumberIDs,
		"message_count", summary.MessageCount,
		"text_message_count", summary.TextMessageCount,
		"status_count", summary.StatusCount,
		"skipped_messages", summary.SkippedMessages,
	)

	if h.cfg.AppSecret != "" {
		if !metaclient.HasSignatureHeader(signature) {
			h.log.Warn("meta webhook rejected: missing X-Hub-Signature-256 header")
			return response.Forbidden(c, "invalid signature")
		}
		if !metaclient.VerifySignature(h.cfg.AppSecret, signature, body) {
			h.log.Warn("meta webhook rejected: signature mismatch",
				"hint", "META_APP_SECRET must be the App Secret from Meta Developer Console, not the access token",
			)
			return response.Forbidden(c, "invalid signature")
		}
		h.log.Info("meta webhook signature valid")
	} else {
		h.log.Warn("meta webhook signature skipped: META_APP_SECRET is empty")
	}

	inboundMessages, err := metaclient.ParseWebhookPayload(body)
	if err != nil {
		h.log.Error("meta webhook parse failed", "error", err)
		return response.BadRequest(c, "invalid webhook payload")
	}

	if len(inboundMessages) == 0 {
		h.log.Info("meta webhook: no processable text messages",
			"note", "delivery/status events or non-text messages are ignored",
		)
		return c.SendStatus(fiber.StatusOK)
	}

	h.log.Info("meta webhook processing messages", "count", len(inboundMessages))

	for _, msg := range inboundMessages {
		h.log.Info("meta inbound message",
			"phone_number_id", msg.PhoneNumberID,
			"from", msg.From,
			"wamid", msg.WAMID,
			"content_preview", metaclient.TruncateForLog(msg.Content, 120),
		)

		if err := h.messages.HandleMetaInbound(c.Context(), msg); err != nil {
			h.log.Error("meta inbound failed",
				"error", err,
				"phone_number_id", msg.PhoneNumberID,
				"from", msg.From,
				"wamid", msg.WAMID,
			)
			continue
		}

		h.log.Info("meta inbound processed",
			"phone_number_id", msg.PhoneNumberID,
			"from", msg.From,
			"wamid", msg.WAMID,
		)
	}

	return c.SendStatus(fiber.StatusOK)
}
