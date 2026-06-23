package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/auth"
	"github.com/omnichannel/omni-chan-v2/internal/user"
)

type AuthHandler struct {
	svc *auth.Service
}

func NewAuthHandler(svc *auth.Service) *AuthHandler {
	return &AuthHandler{svc: svc}
}

type registerRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	TeamName string `json:"team_name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req registerRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if req.Name == "" || req.Email == "" || req.Password == "" {
		return response.BadRequest(c, "name, email, and password are required")
	}
	session, err := h.svc.Register(c.Context(), auth.RegisterInput{
		Name:     req.Name,
		Email:    req.Email,
		Password: req.Password,
		TeamName: req.TeamName,
	})
	if err != nil {
		return h.mapError(c, err)
	}
	return response.Created(c, "registered", sessionResponse(session))
}

func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if req.Email == "" || req.Password == "" {
		return response.BadRequest(c, "email and password are required")
	}
	session, err := h.svc.Login(c.Context(), auth.LoginInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "logged in", sessionResponse(session))
}

func (h *AuthHandler) Refresh(c *fiber.Ctx) error {
	var req refreshRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if req.RefreshToken == "" {
		return response.BadRequest(c, "refresh_token is required")
	}
	session, err := h.svc.Refresh(c.Context(), req.RefreshToken)
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "token refreshed", sessionResponse(session))
}

func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	var req logoutRequest
	_ = c.BodyParser(&req)
	if err := h.svc.Logout(c.Context(), req.RefreshToken); err != nil {
		return response.InternalError(c, "logout failed")
	}
	return response.OK(c, "logged out", nil)
}

func (h *AuthHandler) Me(c *fiber.Ctx) error {
	actor, err := requireActor(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}
	me, err := h.svc.Me(c.Context(), actor.UserID, actor.TeamID, actor.Role)
	if err != nil {
		return h.mapError(c, err)
	}
	return response.OK(c, "ok", me)
}

func (h *AuthHandler) mapError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		return response.Unauthorized(c, err.Error())
	case errors.Is(err, auth.ErrInvalidRefresh):
		return response.Unauthorized(c, err.Error())
	case errors.Is(err, auth.ErrWeakPassword):
		return response.BadRequest(c, err.Error())
	case errors.Is(err, user.ErrEmailTaken):
		return response.BadRequest(c, err.Error())
	default:
		return mapError(c, err)
	}
}

func sessionResponse(session *auth.Session) fiber.Map {
	return fiber.Map{
		"user": fiber.Map{
			"id":    session.UserID,
			"email": session.Email,
			"name":  session.Name,
		},
		"team": fiber.Map{
			"id":   session.TeamID,
			"name": session.TeamName,
			"slug": session.TeamSlug,
		},
		"role":          session.Role,
		"access_token":  session.AccessToken,
		"refresh_token": session.RefreshToken,
	}
}
