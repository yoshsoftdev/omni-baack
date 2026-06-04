package team

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
)

func RegisterRoutes(router fiber.Router, handler *Handler, jwt *jwtmanager.Manager) {
	teams := router.Group("/teams", middleware.Auth(jwt))
	teams.Get("/", handler.Get)
	teams.Patch("/", handler.Update)
	teams.Get("/members", handler.ListMembers)
	teams.Post("/members", handler.AddMember)
	teams.Patch("/members/:id", handler.UpdateMemberRole)
	teams.Delete("/members/:id", handler.RemoveMember)
}
