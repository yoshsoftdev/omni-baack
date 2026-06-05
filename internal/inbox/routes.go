package inbox

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
)

func RegisterRoutes(router fiber.Router, handler *Handler, jwt *jwtmanager.Manager) {
	inbox := router.Group("/inbox", middleware.Auth(jwt))
	inbox.Get("/messages", handler.List)
	inbox.Post("/messages", handler.Send)
}
