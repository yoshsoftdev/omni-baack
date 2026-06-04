package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	jwtmanager "github.com/omnichannel/omni-chan-v2/pkg/jwt"
)

const claimsKey = "auth_claims"

func Auth(jwt *jwtmanager.Manager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		header := c.Get("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			return response.Unauthorized(c, "missing authorization header")
		}

		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		claims, err := jwt.ParseAccess(token)
		if err != nil {
			return response.Unauthorized(c, "invalid token")
		}

		c.Locals(claimsKey, claims)
		return c.Next()
	}
}

func ClaimsFromContext(c *fiber.Ctx) (*jwtmanager.Claims, error) {
	value := c.Locals(claimsKey)
	claims, ok := value.(*jwtmanager.Claims)
	if !ok || claims == nil {
		return nil, fiber.ErrUnauthorized
	}
	return claims, nil
}
