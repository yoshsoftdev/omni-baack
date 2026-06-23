package rbac

import (
	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
)

const PermissionDeniedMessage = "permission denied"

func RequirePermission(service *Service, code string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, err := claimsFromContext(c)
		if err != nil {
			return response.Unauthorized(c, "unauthorized")
		}

		allowed, err := service.HasPermission(c.Context(), claims.Role, code)
		if err != nil {
			return response.InternalError(c, "something went wrong")
		}
		if !allowed {
			return response.Forbidden(c, PermissionDeniedMessage)
		}
		return c.Next()
	}
}

func claimsFromContext(c *fiber.Ctx) (*jwtmanager.Claims, error) {
	value := c.Locals("auth_claims")
	claims, ok := value.(*jwtmanager.Claims)
	if !ok || claims == nil {
		return nil, fiber.ErrUnauthorized
	}
	return claims, nil
}
