package meta

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
)

func RegisterRoutes(router fiber.Router, handler *Handler, jwt *jwtmanager.Manager) {
	meta := router.Group("/meta", middleware.Auth(jwt))
	meta.Get("/embedded-config", handler.EmbeddedConfig)

	webhooks := router.Group("/webhooks/meta")
	webhooks.Get("/", handler.Verify)
	webhooks.Post("/", handler.Receive)
}
