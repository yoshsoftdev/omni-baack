package contact

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/team"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

const maxImportBytes = 5 << 20 // 5MB

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

type contactRequest struct {
	Name   string   `json:"name"`
	Phone  string   `json:"phone"`
	Email  string   `json:"email"`
	Notes  string   `json:"notes"`
	TagIDs []string `json:"tag_ids"`
}

func (h *Handler) List(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	var tagID *uuid.UUID
	if raw := strings.TrimSpace(c.Query("tag_id")); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			return response.BadRequest(c, "invalid tag_id")
		}
		tagID = &parsed
	}

	page := repository.PageParams{
		Page:     c.QueryInt("page", 1),
		PageSize: c.QueryInt("limit", 50),
	}
	result, err := h.service.List(c.Context(), claims.TeamID, claims.UserID, string(claims.Role), ListFilter{
		Search: strings.TrimSpace(c.Query("search")),
		TagID:  tagID,
	}, page)
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "ok", fiber.Map{"contacts": result.Items, "pagination": result.Pagination})
}

func (h *Handler) Get(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	contactID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid contact id")
	}

	contact, err := h.service.Get(c.Context(), claims.TeamID, claims.UserID, string(claims.Role), contactID)
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "ok", contact)
}

func (h *Handler) Create(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	var req contactRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	contact, err := h.service.Create(c.Context(), claims.TeamID, claims.UserID, string(claims.Role), CreateInput{
		Name:  req.Name,
		Phone: req.Phone,
		Email: optionalString(req.Email),
		Notes: optionalString(req.Notes),
	})
	if err != nil {
		return h.mapError(c, err)
	}
	return response.Created(c, "contact created", contact)
}

func (h *Handler) Update(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	contactID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid contact id")
	}

	var req contactRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}

	contact, err := h.service.Update(c.Context(), claims.TeamID, claims.UserID, string(claims.Role), contactID, UpdateInput{
		Name:  optionalString(req.Name),
		Phone: optionalString(req.Phone),
		Email: optionalString(req.Email),
		Notes: optionalString(req.Notes),
	})
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "contact updated", contact)
}

func (h *Handler) Delete(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	contactID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return response.BadRequest(c, "invalid contact id")
	}

	if err := h.service.Delete(c.Context(), claims.TeamID, claims.UserID, string(claims.Role), contactID); err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "contact deleted", nil)
}

func (h *Handler) ImportCSV(c *fiber.Ctx) error {
	return response.Fail(c, fiber.StatusNotImplemented, "contact import pending service migration")
}

func (h *Handler) ImportExcel(c *fiber.Ctx) error {
	return response.Fail(c, fiber.StatusNotImplemented, "contact import pending service migration")
}

func (h *Handler) mapError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, serviceerrors.ErrForbidden):
		return response.Fail(c, fiber.StatusForbidden, err.Error())
	case errors.Is(err, serviceerrors.ErrConflict):
		return response.BadRequest(c, err.Error())
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, team.ErrMemberNotFound):
		return response.NotFound(c, err.Error())
	default:
		if serviceerrors.IsValidation(err) {
			return response.BadRequest(c, err.Error())
		}
		return response.InternalError(c, "something went wrong")
	}
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func parseUUIDs(values []string) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		id, err := uuid.Parse(strings.TrimSpace(value))
		if err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}
