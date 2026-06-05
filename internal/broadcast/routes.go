package broadcast

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
)

func RegisterRoutes(router fiber.Router, handler *Handler, jwt *jwtmanager.Manager) {
	broadcasts := router.Group("/broadcasts", middleware.Auth(jwt))
	broadcasts.Get("/", handler.List)
	broadcasts.Post("/", handler.Create)
	broadcasts.Get("/:id", handler.Get)
	broadcasts.Post("/:id/cancel", handler.Cancel)
}
