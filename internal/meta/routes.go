package meta

import "github.com/gofiber/fiber/v2"

func RegisterRoutes(router fiber.Router, handler *Handler) {
	webhooks := router.Group("/webhooks/meta")
	webhooks.Get("/", handler.Verify)
	webhooks.Post("/", handler.Receive)
}
