package campaign

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
)

func RegisterRoutes(router fiber.Router, handler *Handler, jwt *jwtmanager.Manager) {
	group := router.Group("/campaigns", middleware.Auth(jwt))
	group.Get("/", handler.List)
	group.Get("/reports/overview", handler.Overview)
	group.Post("/", handler.Create)
	group.Post("/:id/send", handler.Send)
	group.Post("/:id/cancel", handler.Cancel)
	group.Get("/:id/report", handler.Report)
}
