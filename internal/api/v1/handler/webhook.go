package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	metapkg "github.com/omnichannel/omni-chan-v2/pkg/meta"
)

type WebhookHandler struct {
	verifyToken string
}

func NewWebhookHandler(verifyToken string) *WebhookHandler {
	return &WebhookHandler{verifyToken: verifyToken}
}

func (h *WebhookHandler) VerifyMeta(c *fiber.Ctx) error {
	result, err := metapkg.VerifyWebhookSubscription(
		c.Query("hub.mode"),
		c.Query("hub.verify_token"),
		c.Query("hub.challenge"),
		h.verifyToken,
	)
	if err != nil {
		return response.Forbidden(c, "webhook verification failed")
	}
	return c.Status(fiber.StatusOK).SendString(result)
}
