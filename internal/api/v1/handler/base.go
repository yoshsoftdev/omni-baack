package handler

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/auth"
	"github.com/omnichannel/omni-chan-v2/pkg/repository"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

func requireActor(c *fiber.Ctx) (auth.Actor, error) {
	return middleware.ActorFromContext(c)
}

func pageParams(c *fiber.Ctx) (repository.PageParams, error) {
	if strings.TrimSpace(c.Query("limit")) != "" {
		return repository.PageParams{}, serviceerrors.NewValidation("limit", "use page_size instead")
	}
	page := c.QueryInt("page", repository.DefaultPage)
	pageSize := c.QueryInt("page_size", repository.DefaultPageSize)
	if page < 1 {
		return repository.PageParams{}, serviceerrors.NewValidation("page", "must be >= 1")
	}
	if pageSize < 1 || pageSize > repository.MaxPageSize {
		return repository.PageParams{}, serviceerrors.NewValidation("page_size", "must be between 1 and 100")
	}
	return repository.PageParams{Page: page, PageSize: pageSize}, nil
}

func parseUUIDParam(c *fiber.Ctx, name string) (uuid.UUID, error) {
	return uuid.Parse(c.Params(name))
}

func parseUUIDFromString(raw string) (uuid.UUID, error) {
	return uuid.Parse(strings.TrimSpace(raw))
}

func optionalUUIDQuery(c *fiber.Ctx, name string) (*uuid.UUID, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func parseTimeQuery(c *fiber.Ctx, name string) (time.Time, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return time.Time{}, errors.New("missing time query")
	}
	return time.Parse(time.RFC3339, raw)
}

func mapError(c *fiber.Ctx, err error) error {
	if err == nil {
		return response.InternalError(c, "internal server error")
	}
	switch {
	case serviceerrors.IsForbidden(err):
		return response.Forbidden(c, err.Error())
	case serviceerrors.IsNotFound(err):
		return response.NotFound(c, err.Error())
	case errors.Is(err, serviceerrors.ErrConflict):
		return response.Conflict(c, err.Error())
	case errors.Is(err, serviceerrors.ErrQuotaExceeded):
		return response.QuotaExceeded(c, err.Error())
	case serviceerrors.IsPaymentRequired(err):
		return response.PaymentRequired(c, err.Error())
	case serviceerrors.IsPermissionRequired(err):
		return response.PermissionRequired(c, err.Error())
	default:
		if serviceerrors.IsValidation(err) {
			return response.BadRequest(c, err.Error())
		}
		return response.InternalError(c, "internal server error")
	}
}
