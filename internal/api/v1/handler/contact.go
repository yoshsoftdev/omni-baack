package handler

import (
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/api/v1/dto"
	"github.com/omnichannel/omni-chan-v2/internal/contact"
)

type ContactHandler struct {
	svc *contact.Service
}

func NewContactHandler(svc *contact.Service) *ContactHandler {
	return &ContactHandler{svc: svc}
}

type createContactRequest struct {
	Name            string                     `json:"name"`
	Phone           string                     `json:"phone"`
	Email           string                     `json:"email"`
	Notes           string                     `json:"notes"`
	City            string                     `json:"city"`
	Country         string                     `json:"country"`
	Province        string                     `json:"province"`
	Company         string                     `json:"company"`
	JobTitle        string                     `json:"job_title"`
	Language        string                     `json:"language"`
	MarketingStatus string                     `json:"marketing_status"`
	ConsentStatus   string                     `json:"consent_status"`
	TagIDs          []string                   `json:"tag_ids"`
	CustomFields    map[string]json.RawMessage `json:"custom_fields"`
}

type updateContactRequest struct {
	Name            *string                    `json:"name"`
	Phone           *string                    `json:"phone"`
	Email           *string                    `json:"email"`
	Notes           *string                    `json:"notes"`
	City            *string                    `json:"city"`
	Country         *string                    `json:"country"`
	Province        *string                    `json:"province"`
	Company         *string                    `json:"company"`
	JobTitle        *string                    `json:"job_title"`
	Language        *string                    `json:"language"`
	MarketingStatus *string                    `json:"marketing_status"`
	ConsentStatus   *string                    `json:"consent_status"`
	TagIDs          []string                   `json:"tag_ids"`
	CustomFields    map[string]json.RawMessage `json:"custom_fields"`
}

type createFieldDefinitionRequest struct {
	FieldKey  string          `json:"field_key"`
	Label     string          `json:"label"`
	FieldType string          `json:"field_type"`
	Options   json.RawMessage `json:"options"`
	SortOrder int             `json:"sort_order"`
}

type updateFieldDefinitionRequest struct {
	Label     *string         `json:"label"`
	FieldType *string         `json:"field_type"`
	Options   json.RawMessage `json:"options"`
	SortOrder *int            `json:"sort_order"`
}

func parseMarketingStatus(raw string) (contact.MarketingStatus, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "subscribed", "opt_in":
		return contact.MarketingSubscribed, true
	case "unsubscribed", "opt_out":
		return contact.MarketingUnsubscribed, true
	case "unknown", "":
		return contact.MarketingUnknown, true
	default:
		return "", false
	}
}

func optionalMarketingStatus(marketing, consent string) (contact.MarketingStatus, error) {
	if strings.TrimSpace(marketing) != "" {
		status, ok := parseMarketingStatus(marketing)
		if !ok {
			return "", fiber.NewError(fiber.StatusBadRequest, "invalid marketing_status")
		}
		return status, nil
	}
	if strings.TrimSpace(consent) != "" {
		status, ok := parseMarketingStatus(consent)
		if !ok {
			return "", fiber.NewError(fiber.StatusBadRequest, "invalid consent_status")
		}
		return status, nil
	}
	return contact.MarketingUnknown, nil
}

func optionalMarketingStatusPtr(marketing, consent *string) (*contact.MarketingStatus, error) {
	marketingRaw := ""
	if marketing != nil {
		marketingRaw = *marketing
	}
	consentRaw := ""
	if consent != nil {
		consentRaw = *consent
	}
	if marketingRaw == "" && consentRaw == "" {
		return nil, nil
	}
	status, err := optionalMarketingStatus(marketingRaw, consentRaw)
	if err != nil {
		return nil, err
	}
	return &status, nil
}

func parseContactTagIDs(values []string) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		id, err := uuid.Parse(strings.TrimSpace(value))
		if err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

func (h *ContactHandler) List(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	tagID, err := optionalUUIDQuery(c, "tag_id")
	if err != nil {
		return response.BadRequest(c, "invalid tag_id")
	}
	var marketingStatus *contact.MarketingStatus
	if raw := strings.TrimSpace(c.Query("consent_status")); raw != "" {
		status, ok := parseMarketingStatus(raw)
		if !ok {
			return response.BadRequest(c, "invalid consent_status")
		}
		marketingStatus = &status
	} else if raw := strings.TrimSpace(c.Query("marketing_status")); raw != "" {
		status, ok := parseMarketingStatus(raw)
		if !ok {
			return response.BadRequest(c, "invalid marketing_status")
		}
		marketingStatus = &status
	}
	params, err := pageParams(c)
	if err != nil {
		return mapError(c, err)
	}
	page, err := h.svc.List(c.Context(), actor.TeamID, actor.UserID, actor.Role, contact.ListFilter{
		Search:          strings.TrimSpace(c.Query("search")),
		TagID:           tagID,
		MarketingStatus: marketingStatus,
	}, params)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", dto.NewPageResponse(page))
}

func (h *ContactHandler) Stats(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	stats, err := h.svc.Stats(c.Context(), actor.TeamID, actor.UserID, actor.Role)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", stats)
}

func (h *ContactHandler) Get(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid contact id")
	}
	ct, err := h.svc.Get(c.Context(), actor.TeamID, actor.UserID, actor.Role, id)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", ct)
}

func (h *ContactHandler) Create(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	var req createContactRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	marketingStatus, err := optionalMarketingStatus(req.MarketingStatus, req.ConsentStatus)
	if err != nil {
		return response.BadRequest(c, err.Error())
	}
	ct, err := h.svc.Create(c.Context(), actor.TeamID, actor.UserID, actor.Role, contact.CreateInput{
		Name:            req.Name,
		Phone:           req.Phone,
		Email:           optionalString(req.Email),
		Notes:           optionalString(req.Notes),
		City:            optionalString(req.City),
		Country:         optionalString(req.Country),
		Province:        optionalString(req.Province),
		Company:         optionalString(req.Company),
		JobTitle:        optionalString(req.JobTitle),
		Language:        optionalString(req.Language),
		Source:          contact.SourceManual,
		MarketingStatus: marketingStatus,
		TagIDs:          parseContactTagIDs(req.TagIDs),
		CustomFields:    req.CustomFields,
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.Created(c, "contact created", ct)
}

func (h *ContactHandler) Update(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid contact id")
	}
	var req updateContactRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	marketingStatus, err := optionalMarketingStatusPtr(req.MarketingStatus, req.ConsentStatus)
	if err != nil {
		return response.BadRequest(c, err.Error())
	}
	input := contact.UpdateInput{
		Name:            req.Name,
		Phone:           req.Phone,
		Email:           req.Email,
		Notes:           req.Notes,
		City:            req.City,
		Country:         req.Country,
		Province:        req.Province,
		Company:         req.Company,
		JobTitle:        req.JobTitle,
		Language:        req.Language,
		MarketingStatus: marketingStatus,
	}
	if req.TagIDs != nil {
		input.TagIDs = parseContactTagIDs(req.TagIDs)
		input.TagIDsSet = true
	}
	if req.CustomFields != nil {
		input.CustomFields = req.CustomFields
		input.CustomFieldsSet = true
	}
	ct, err := h.svc.Update(c.Context(), actor.TeamID, actor.UserID, actor.Role, id, input)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "contact updated", ct)
}

func (h *ContactHandler) Delete(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid contact id")
	}
	if err := h.svc.Delete(c.Context(), actor.TeamID, actor.UserID, actor.Role, id); err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "contact deleted", nil)
}

func (h *ContactHandler) ListFieldDefinitions(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	items, err := h.svc.ListFieldDefinitions(c.Context(), actor.TeamID, actor.UserID, actor.Role)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", fiber.Map{"items": items})
}

func (h *ContactHandler) CreateFieldDefinition(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	var req createFieldDefinitionRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	def, err := h.svc.CreateFieldDefinition(c.Context(), actor.TeamID, actor.UserID, actor.Role, contact.CreateFieldDefinitionInput{
		FieldKey:  req.FieldKey,
		Label:     req.Label,
		FieldType: contact.FieldType(strings.TrimSpace(req.FieldType)),
		Options:   req.Options,
		SortOrder: req.SortOrder,
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.Created(c, "field definition created", def)
}

func (h *ContactHandler) UpdateFieldDefinition(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid field definition id")
	}
	var req updateFieldDefinitionRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	var fieldType *contact.FieldType
	if req.FieldType != nil {
		ft := contact.FieldType(strings.TrimSpace(*req.FieldType))
		fieldType = &ft
	}
	def, err := h.svc.UpdateFieldDefinition(c.Context(), actor.TeamID, actor.UserID, actor.Role, id, contact.UpdateFieldDefinitionInput{
		Label:     req.Label,
		FieldType: fieldType,
		Options:   req.Options,
		SortOrder: req.SortOrder,
	})
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "field definition updated", def)
}

func (h *ContactHandler) DeleteFieldDefinition(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	id, err := parseUUIDParam(c, "id")
	if err != nil {
		return response.BadRequest(c, "invalid field definition id")
	}
	if err := h.svc.DeleteFieldDefinition(c.Context(), actor.TeamID, actor.UserID, actor.Role, id); err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "field definition deleted", nil)
}
