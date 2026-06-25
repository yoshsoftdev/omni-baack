package handler

import (
	"strings"

	"github.com/gofiber/fiber/v2"
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
	Name            string `json:"name"`
	Phone           string `json:"phone"`
	Email           string `json:"email"`
	Notes           string `json:"notes"`
	City            string `json:"city"`
	Language        string `json:"language"`
	MarketingStatus string `json:"marketing_status"`
	ConsentStatus   string `json:"consent_status"`
}

type updateContactRequest struct {
	Name            *string `json:"name"`
	Phone           *string `json:"phone"`
	Email           *string `json:"email"`
	Notes           *string `json:"notes"`
	City            *string `json:"city"`
	Language        *string `json:"language"`
	MarketingStatus *string `json:"marketing_status"`
	ConsentStatus   *string `json:"consent_status"`
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

func (h *ContactHandler) List(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	tagID, err := optionalUUIDQuery(c, "tag_id")
	if err != nil {
		return response.BadRequest(c, "invalid tag_id")
	}
	params, err := pageParams(c)
	if err != nil {
		return mapError(c, err)
	}
	page, err := h.svc.List(c.Context(), actor.TeamID, actor.UserID, actor.Role, contact.ListFilter{
		Search: strings.TrimSpace(c.Query("search")),
		TagID:  tagID,
	}, params)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", dto.NewPageResponse(page))
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
		Language:        optionalString(req.Language),
		Source:          contact.SourceManual,
		MarketingStatus: marketingStatus,
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
	ct, err := h.svc.Update(c.Context(), actor.TeamID, actor.UserID, actor.Role, id, contact.UpdateInput{
		Name:            req.Name,
		Phone:           req.Phone,
		Email:           req.Email,
		Notes:           req.Notes,
		City:            req.City,
		Language:        req.Language,
		MarketingStatus: marketingStatus,
	})
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
