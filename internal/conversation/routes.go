package conversation

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
)

func RegisterRoutes(router fiber.Router, handler *Handler, jwt *jwtmanager.Manager) {
	conversations := router.Group("/conversations", middleware.Auth(jwt))
	conversations.Get("/", handler.List)
	conversations.Get("/unread-count", handler.UnreadCount)
	conversations.Get("/:id", handler.Get)
	conversations.Patch("/:id/assign", handler.Assign)
	conversations.Patch("/:id/status", handler.UpdateStatus)
	conversations.Post("/:id/read", handler.MarkRead)
}
