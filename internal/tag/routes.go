package tag

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
)

func RegisterRoutes(router fiber.Router, handler *Handler, jwt *jwtmanager.Manager) {
	tags := router.Group("/tags", middleware.Auth(jwt))
	tags.Get("/", handler.List)
	tags.Post("/", handler.Create)
	tags.Patch("/:id", handler.Update)
	tags.Delete("/:id", handler.Delete)
}
