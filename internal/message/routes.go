package message

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
)

func RegisterRoutes(router fiber.Router, handler *Handler, jwt *jwtmanager.Manager) {
	conversations := router.Group("/conversations", middleware.Auth(jwt))
	conversations.Get("/:id/messages", handler.List)
	conversations.Post("/:id/messages", handler.Send)
}
