package v1

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/auth"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
)

type Deps struct {
	AuthHandler  *auth.Handler
	TeamHandler  *team.Handler
	JWT          *jwtmanager.Manager
}

func Register(router fiber.Router, deps Deps) {
	router.Get("/health", health)

	if deps.JWT == nil {
		return
	}
	if deps.AuthHandler != nil {
		auth.RegisterRoutes(router, deps.AuthHandler, deps.JWT)
	}
	if deps.TeamHandler != nil {
		team.RegisterRoutes(router, deps.TeamHandler, deps.JWT)
	}
}

func health(c *fiber.Ctx) error {
	return response.OK(c, "ok", fiber.Map{
		"version": "v1",
	})
}
