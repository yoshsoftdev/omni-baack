package otp

import (
	"crypto/subtle"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
)

var ErrInvalidToken = errors.New("invalid token")

type Handler struct {
	service     *Service
	verifyToken string
}

func NewHandler(service *Service, verifyToken string) *Handler {
	return &Handler{
		service:     service,
		verifyToken: strings.TrimSpace(verifyToken),
	}
}

type sendRequest struct {
	Token    string `json:"token"`
	DeviceID string `json:"device_id"`
	Phone    string `json:"phone"`
	Code     string `json:"code"`
}

func (h *Handler) Send(c *fiber.Ctx) error {
	var req sendRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	if err := h.validateToken(req.Token); err != nil {
		return response.Unauthorized(c, err.Error())
	}

	result, err := h.service.Send(c.Context(), SendInput{
		DeviceID: req.DeviceID,
		Phone:    req.Phone,
		Code:     req.Code,
	})
	if err != nil {
		return h.mapError(c, err)
	}

	return response.Created(c, "otp sent", result)
}

func (h *Handler) validateToken(token string) error {
	if h.verifyToken == "" {
		return ErrInvalidToken
	}
	provided := strings.TrimSpace(token)
	if provided == "" {
		return ErrInvalidToken
	}
	if subtle.ConstantTimeCompare([]byte(provided), []byte(h.verifyToken)) != 1 {
		return ErrInvalidToken
	}
	return nil
}

func (h *Handler) mapError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrPhoneRequired),
		errors.Is(err, ErrPhoneInvalid),
		errors.Is(err, ErrCodeRequired),
		errors.Is(err, ErrCodeInvalid):
		return response.BadRequest(c, err.Error())
	case errors.Is(err, ErrDeviceNotFound):
		return response.NotFound(c, err.Error())
	case errors.Is(err, ErrDeviceNotConnected):
		return response.BadRequest(c, err.Error())
	default:
		return response.InternalError(c, err.Error())
	}
}
