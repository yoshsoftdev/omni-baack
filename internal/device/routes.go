package device

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
)

func RegisterRoutes(router fiber.Router, handler *Handler, jwt *jwtmanager.Manager) {
	devices := router.Group("/devices", middleware.Auth(jwt))
	devices.Get("/", handler.List)
	devices.Post("/", handler.Create)
	devices.Get("/:id", handler.Get)
	devices.Patch("/:id", handler.Update)
	devices.Delete("/:id", handler.Delete)
	devices.Post("/:id/connect", handler.Connect)
	devices.Post("/:id/disconnect", handler.Disconnect)
}
