package response

import "github.com/gofiber/fiber/v2"

type Body struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func OK(c *fiber.Ctx, message string, data interface{}) error {
	return c.JSON(Body{
		Success: true,
		Message: message,
		Data:    data,
	})
}

func Created(c *fiber.Ctx, message string, data interface{}) error {
	return c.Status(fiber.StatusCreated).JSON(Body{
		Success: true,
		Message: message,
		Data:    data,
	})
}

func Accepted(c *fiber.Ctx, message string, data interface{}) error {
	return c.Status(fiber.StatusAccepted).JSON(Body{
		Success: true,
		Message: message,
		Data:    data,
	})
}

func Fail(c *fiber.Ctx, status int, message string) error {
	return c.Status(status).JSON(Body{
		Success: false,
		Message: message,
	})
}

func BadRequest(c *fiber.Ctx, message string) error {
	return Fail(c, fiber.StatusBadRequest, message)
}

func Unauthorized(c *fiber.Ctx, message string) error {
	return Fail(c, fiber.StatusUnauthorized, message)
}

func NotFound(c *fiber.Ctx, message string) error {
	return Fail(c, fiber.StatusNotFound, message)
}

func InternalError(c *fiber.Ctx, message string) error {
	return Fail(c, fiber.StatusInternalServerError, message)
}

func Forbidden(c *fiber.Ctx, message string) error {
	return Fail(c, fiber.StatusForbidden, message)
}
