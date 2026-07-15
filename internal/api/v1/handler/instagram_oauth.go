package handler

import (
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/instagram"
	"github.com/omnichannel/omni-chan-v2/pkg/serviceerrors"
)

type InstagramOAuthHandler struct {
	svc *instagram.Service
}

func NewInstagramOAuthHandler(svc *instagram.Service) *InstagramOAuthHandler {
	return &InstagramOAuthHandler{svc: svc}
}

func (h *InstagramOAuthHandler) ConnectURL(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	result, err := h.svc.ConnectURL(c.Context(), actor.TeamID, actor.UserID, actor.Role, strings.TrimSpace(c.Query("return_url")))
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", result)
}

func (h *InstagramOAuthHandler) Callback(c *fiber.Ctx) error {
	code := strings.TrimSpace(c.Query("code"))
	state := strings.TrimSpace(c.Query("state"))
	if errParam := strings.TrimSpace(c.Query("error")); errParam != "" {
		desc := strings.TrimSpace(c.Query("error_description"))
		if desc == "" {
			desc = errParam
		}
		return h.finishCallback(c, "", serviceerrors.NewValidation("instagram", desc))
	}

	result, returnURL, err := h.svc.HandleCallback(c.Context(), code, state)
	if err != nil {
		return h.finishCallback(c, returnURL, err)
	}
	if returnURL != "" {
		target, buildErr := buildInstagramSuccessRedirect(returnURL, result)
		if buildErr == nil {
			return c.Redirect(target, fiber.StatusFound)
		}
	}
	return response.OK(c, "instagram connected", result)
}

func (h *InstagramOAuthHandler) Status(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	result, err := h.svc.Status(c.Context(), actor.TeamID, actor.UserID, actor.Role)
	if err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "ok", result)
}

func (h *InstagramOAuthHandler) Disconnect(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	if err := h.svc.Disconnect(c.Context(), actor.TeamID, actor.UserID, actor.Role); err != nil {
		return mapError(c, err)
	}
	return response.OK(c, "instagram disconnected", nil)
}

func (h *InstagramOAuthHandler) finishCallback(c *fiber.Ctx, returnURL string, err error) error {
	if returnURL != "" {
		target, buildErr := buildInstagramErrorRedirect(returnURL, err)
		if buildErr == nil {
			return c.Redirect(target, fiber.StatusFound)
		}
	}
	return mapError(c, err)
}

func buildInstagramSuccessRedirect(returnURL string, result *instagram.CallbackResult) (string, error) {
	u, err := url.Parse(returnURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("instagram", "connected")
	if result != nil {
		if result.Username != "" {
			q.Set("username", result.Username)
		}
		if result.AccountType != "" {
			q.Set("account_type", result.AccountType)
		}
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func buildInstagramErrorRedirect(returnURL string, err error) (string, error) {
	u, errParse := url.Parse(returnURL)
	if errParse != nil {
		return "", errParse
	}
	q := u.Query()
	q.Set("instagram", "error")
	if err != nil {
		q.Set("message", err.Error())
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
