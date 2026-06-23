package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/auth"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

const actorKey = "auth_actor"

// ResolveActor builds an auth.Actor from JWT claims and stores it on the request context.
func ResolveActor(resolver auth.ActorResolver) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, err := ClaimsFromContext(c)
		if err != nil {
			return response.Unauthorized(c, "unauthorized")
		}
		actor, err := resolver.Resolve(c.Context(), claims)
		if err != nil {
			if serviceerrors.IsForbidden(err) {
				return response.Forbidden(c, err.Error())
			}
			return response.Unauthorized(c, "unauthorized")
		}
		c.Locals(actorKey, actor)
		c.SetUserContext(auth.WithActor(c.UserContext(), actor))
		return c.Next()
	}
}

// ActorFromContext returns the resolved actor for the current request.
func ActorFromContext(c *fiber.Ctx) (auth.Actor, error) {
	value := c.Locals(actorKey)
	actor, ok := value.(auth.Actor)
	if !ok {
		return auth.Actor{}, fiber.ErrUnauthorized
	}
	return actor, nil
}
