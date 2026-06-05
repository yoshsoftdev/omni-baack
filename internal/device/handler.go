package device

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/team"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type createDeviceRequest struct {
	Name string `json:"name"`
}

type updateDeviceRequest struct {
	Name string `json:"name"`
}

type connectDeviceRequest struct {
	PhoneNumberID string `json:"phone_number_id"`
	WabaID        string `json:"waba_id"`
	AccessToken   string `json:"access_token"`
}

func (h *Handler) List(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	devices, err := h.service.List(c.Context(), claims.TeamID, claims.UserID)
	if err != nil {
		return h.mapError(c, err)
	}

	items := make([]Response, 0, len(devices))
	for i := range devices {
		items = append(items, ToResponse(&devices[i]))
	}

	return response.OK(c, "ok", fiber.Map{"devices": items})
}

func (h *Handler) Get(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	deviceID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid device id")
	}

	device, err := h.service.Get(c.Context(), claims.TeamID, claims.UserID, deviceID)
	if err != nil {
		return h.mapError(c, err)
	}

	return response.OK(c, "ok", ToResponse(device))
}

func (h *Handler) Create(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	var req createDeviceRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	device, err := h.service.Create(c.Context(), claims.TeamID, claims.UserID, team.Role(claims.Role), CreateInput{
		Name: req.Name,
	})
	if err != nil {
		return h.mapError(c, err)
	}

	return response.Created(c, "device created", ToResponse(device))
}

func (h *Handler) Update(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	deviceID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid device id")
	}

	var req updateDeviceRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	device, err := h.service.Update(c.Context(), claims.TeamID, claims.UserID, team.Role(claims.Role), deviceID, UpdateInput{
		Name: req.Name,
	})
	if err != nil {
		return h.mapError(c, err)
	}

	return response.OK(c, "device updated", ToResponse(device))
}

func (h *Handler) Delete(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	deviceID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid device id")
	}

	if err := h.service.Delete(c.Context(), claims.TeamID, claims.UserID, team.Role(claims.Role), deviceID); err != nil {
		return h.mapError(c, err)
	}

	return response.OK(c, "device deleted", nil)
}

func (h *Handler) Connect(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	deviceID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid device id")
	}

	var req connectDeviceRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	device, err := h.service.Connect(c.Context(), claims.TeamID, claims.UserID, team.Role(claims.Role), deviceID, ConnectInput{
		PhoneNumberID: req.PhoneNumberID,
		WabaID:        req.WabaID,
		AccessToken:   req.AccessToken,
	})
	if err != nil {
		return h.mapError(c, err)
	}

	return response.OK(c, "device connected", ToResponse(device))
}

func (h *Handler) Disconnect(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	deviceID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid device id")
	}

	device, err := h.service.Disconnect(c.Context(), claims.TeamID, claims.UserID, team.Role(claims.Role), deviceID)
	if err != nil {
		return h.mapError(c, err)
	}

	return response.OK(c, "device disconnected", ToResponse(device))
}

func (h *Handler) mapError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrForbidden):
		return response.Fail(c, fiber.StatusForbidden, err.Error())
	case errors.Is(err, ErrNameRequired), errors.Is(err, ErrAlreadyConnected), errors.Is(err, ErrMetaRequired), errors.Is(err, ErrMetaInvalid):
		return response.BadRequest(c, err.Error())
	case errors.Is(err, ErrNotFound), errors.Is(err, team.ErrMemberNotFound):
		return response.NotFound(c, err.Error())
	default:
		if strings.Contains(err.Error(), "already registered") {
			return response.BadRequest(c, err.Error())
		}
		return response.InternalError(c, "something went wrong")
	}
}
