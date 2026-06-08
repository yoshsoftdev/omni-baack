package template

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
)

func RegisterRoutes(router fiber.Router, handler *Handler, jwt *jwtmanager.Manager) {
	group := router.Group("/templates", middleware.Auth(jwt))
	group.Get("/", handler.List)
	group.Post("/sync", handler.Sync)
}
