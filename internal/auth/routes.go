package auth

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
)

func RegisterRoutes(router fiber.Router, handler *Handler, jwt *jwtmanager.Manager) {
	auth := router.Group("/auth")
	auth.Post("/register", handler.Register)
	auth.Post("/login", handler.Login)
	auth.Post("/refresh", handler.Refresh)
	auth.Post("/logout", handler.Logout)

	protected := auth.Group("", middleware.Auth(jwt))
	protected.Get("/me", handler.Me)
}
