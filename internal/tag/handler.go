//go:build legacy

package tag

import (
	"errors"

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

type tagRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (h *Handler) List(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	tags, err := h.service.List(c.Context(), claims.TeamID, claims.UserID)
	if err != nil {
		return h.mapError(c, err)
	}

	items := make([]Response, 0, len(tags))
	for i := range tags {
		items = append(items, ToResponse(&tags[i]))
	}
	return response.OK(c, "ok", fiber.Map{"tags": items})
}

func (h *Handler) Create(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	var req tagRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	tag, err := h.service.Create(c.Context(), claims.TeamID, claims.UserID, team.Role(claims.Role), CreateInput{
		Name:  req.Name,
		Color: req.Color,
	})
	if err != nil {
		return h.mapError(c, err)
	}
	return response.Created(c, "tag created", ToResponse(tag))
}

func (h *Handler) Update(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	tagID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid tag id")
	}

	var req tagRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	tag, err := h.service.Update(c.Context(), claims.TeamID, claims.UserID, team.Role(claims.Role), tagID, UpdateInput{
		Name:  req.Name,
		Color: req.Color,
	})
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "tag updated", ToResponse(tag))
}

func (h *Handler) Delete(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	tagID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid tag id")
	}

	if err := h.service.Delete(c.Context(), claims.TeamID, claims.UserID, team.Role(claims.Role), tagID); err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "tag deleted", nil)
}

func (h *Handler) mapError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrForbidden):
		return response.Fail(c, fiber.StatusForbidden, err.Error())
	case errors.Is(err, ErrNameRequired):
		return response.BadRequest(c, err.Error())
	case errors.Is(err, ErrNotFound), errors.Is(err, team.ErrMemberNotFound):
		return response.NotFound(c, err.Error())
	default:
		if err != nil && err.Error() == "tag name already exists" {
			return response.BadRequest(c, err.Error())
		}
		return response.InternalError(c, "something went wrong")
	}
}
