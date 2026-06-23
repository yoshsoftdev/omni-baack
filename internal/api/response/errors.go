package response

import "github.com/gofiber/fiber/v2"

// API error codes for v1 failure responses.
const (
	CodeValidationError      = "validation_error"
	CodeUnauthorized         = "unauthorized"
	CodeForbidden            = "forbidden"
	CodeNotFound             = "not_found"
	CodeConflict             = "conflict"
	CodeQuotaExceeded        = "quota_exceeded"
	CodeFeatureNotAvailable  = "feature_not_available"
	CodeInternalError        = "internal_error"
)

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type FailBody struct {
	Success bool        `json:"success"`
	Error   ErrorDetail `json:"error"`
}

func fail(c *fiber.Ctx, status int, code, message string) error {
	return c.Status(status).JSON(FailBody{
		Success: false,
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

func BadRequest(c *fiber.Ctx, message string) error {
	return fail(c, fiber.StatusBadRequest, CodeValidationError, message)
}

func Unauthorized(c *fiber.Ctx, message string) error {
	return fail(c, fiber.StatusUnauthorized, CodeUnauthorized, message)
}

func Forbidden(c *fiber.Ctx, message string) error {
	return fail(c, fiber.StatusForbidden, CodeForbidden, message)
}

func NotFound(c *fiber.Ctx, message string) error {
	return fail(c, fiber.StatusNotFound, CodeNotFound, message)
}

func Conflict(c *fiber.Ctx, message string) error {
	return fail(c, fiber.StatusConflict, CodeConflict, message)
}

func QuotaExceeded(c *fiber.Ctx, message string) error {
	return fail(c, fiber.StatusPaymentRequired, CodeQuotaExceeded, message)
}

func FeatureNotAvailable(c *fiber.Ctx, message string) error {
	return fail(c, fiber.StatusNotImplemented, CodeFeatureNotAvailable, message)
}

func InternalError(c *fiber.Ctx, message string) error {
	return fail(c, fiber.StatusInternalServerError, CodeInternalError, message)
}
