package inbox

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type sendMessageRequest struct {
	DeviceID  string `json:"device_id"`
	Recipient string `json:"recipient"`
	Content   string `json:"content"`
}

func (h *Handler) List(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	messages, err := h.service.List(c.Context(), claims.TeamID)
	if err != nil {
		return response.InternalError(c, "something went wrong")
	}

	return response.OK(c, "ok", fiber.Map{"messages": messages})
}

func (h *Handler) Send(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	var req sendMessageRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	deviceID, err := uuid.Parse(req.DeviceID)
	if err != nil {
		return response.BadRequest(c, "invalid device id")
	}

	msg, err := h.service.Send(c.Context(), claims.TeamID, SendInput{
		DeviceID:  deviceID,
		Recipient: req.Recipient,
		Content:   req.Content,
	})
	if err != nil {
		return h.mapError(c, err)
	}

	return response.Created(c, "message sent", msg)
}

func (h *Handler) mapError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrInvalidContent), errors.Is(err, ErrInvalidRecipient):
		return response.BadRequest(c, err.Error())
	case errors.Is(err, ErrDeviceNotConnected), errors.Is(err, ErrDeviceNotFound):
		return response.NotFound(c, err.Error())
	default:
		return response.InternalError(c, err.Error())
	}
}
