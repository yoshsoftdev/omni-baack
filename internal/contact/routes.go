package contact

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
)

func RegisterRoutes(router fiber.Router, handler *Handler, jwt *jwtmanager.Manager) {
	contacts := router.Group("/contacts", middleware.Auth(jwt))
	contacts.Get("/", handler.List)
	contacts.Post("/", handler.Create)
	contacts.Post("/import/csv", handler.ImportCSV)
	contacts.Post("/import/excel", handler.ImportExcel)
	contacts.Get("/:id", handler.Get)
	contacts.Patch("/:id", handler.Update)
	contacts.Delete("/:id", handler.Delete)
}
