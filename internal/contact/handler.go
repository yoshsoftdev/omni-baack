package contact

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/team"
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

	contacts, err := h.service.List(c.Context(), claims.TeamID, claims.UserID, ListInput{
		Search: strings.TrimSpace(c.Query("search")),
		TagID:  tagID,
		Limit:  c.QueryInt("limit", 50),
		Offset: c.QueryInt("offset", 0),
	})
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "ok", fiber.Map{"contacts": contacts})
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

	contact, err := h.service.Get(c.Context(), claims.TeamID, claims.UserID, contactID)
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

	contact, err := h.service.Create(c.Context(), claims.TeamID, claims.UserID, team.Role(claims.Role), CreateInput{
		Name:   req.Name,
		Phone:  req.Phone,
		Email:  req.Email,
		Notes:  req.Notes,
		TagIDs: parseUUIDs(req.TagIDs),
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

	contact, err := h.service.Update(c.Context(), claims.TeamID, claims.UserID, team.Role(claims.Role), contactID, UpdateInput{
		Name:   req.Name,
		Phone:  req.Phone,
		Email:  req.Email,
		Notes:  req.Notes,
		TagIDs: parseUUIDs(req.TagIDs),
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

	if err := h.service.Delete(c.Context(), claims.TeamID, claims.UserID, team.Role(claims.Role), contactID); err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "contact deleted", nil)
}

func (h *Handler) ImportCSV(c *fiber.Ctx) error {
	return h.importFile(c, ParseCSV)
}

func (h *Handler) ImportExcel(c *fiber.Ctx) error {
	return h.importFile(c, ParseExcel)
}

func (h *Handler) importFile(c *fiber.Ctx, parser func([]byte) ([]ImportRow, error)) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	file, err := c.FormFile("file")
	if err != nil {
		return response.BadRequest(c, "file is required")
	}

	src, err := file.Open()
	if err != nil {
		return response.BadRequest(c, "cannot open file")
	}
	defer src.Close()

	data, err := ReadUploadFile(src, maxImportBytes)
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	rows, err := parser(data)
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	result, err := h.service.ImportRows(c.Context(), claims.TeamID, claims.UserID, team.Role(claims.Role), rows)
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "import completed", result)
}

func (h *Handler) mapError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrForbidden):
		return response.Fail(c, fiber.StatusForbidden, err.Error())
	case errors.Is(err, ErrNameRequired), errors.Is(err, ErrPhoneRequired),
		errors.Is(err, ErrPhoneInvalid), errors.Is(err, ErrInvalidTag), errors.Is(err, ErrEmptyImport):
		return response.BadRequest(c, err.Error())
	case errors.Is(err, ErrNotFound), errors.Is(err, team.ErrMemberNotFound):
		return response.NotFound(c, err.Error())
	default:
		if err != nil && (strings.Contains(err.Error(), "already exists")) {
			return response.BadRequest(c, err.Error())
		}
		return response.InternalError(c, "something went wrong")
	}
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
