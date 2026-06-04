package auth

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/omnichannel/omni-chan-v2/internal/api/middleware"
	"github.com/omnichannel/omni-chan-v2/internal/api/response"
	"github.com/omnichannel/omni-chan-v2/internal/user"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
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

func (h *Handler) Register(c *fiber.Ctx) error {
	var req registerRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if req.Name == "" || req.Email == "" || req.Password == "" {
		return response.BadRequest(c, "name, email, and password are required")
	}

	session, err := h.service.Register(c.Context(), RegisterInput{
		Name:     req.Name,
		Email:    req.Email,
		Password: req.Password,
		TeamName: req.TeamName,
	})
	if err != nil {
		return h.mapError(c, err)
	}

	return response.Created(c, "registered", toSessionResponse(session))
}

func (h *Handler) Login(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if req.Email == "" || req.Password == "" {
		return response.BadRequest(c, "email and password are required")
	}

	session, err := h.service.Login(c.Context(), LoginInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		return h.mapError(c, err)
	}

	return response.OK(c, "logged in", toSessionResponse(session))
}

func (h *Handler) Refresh(c *fiber.Ctx) error {
	var req refreshRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "invalid request body")
	}
	if req.RefreshToken == "" {
		return response.BadRequest(c, "refresh_token is required")
	}

	session, err := h.service.Refresh(c.Context(), req.RefreshToken)
	if err != nil {
		return h.mapError(c, err)
	}

	return response.OK(c, "token refreshed", toSessionResponse(session))
}

func (h *Handler) Logout(c *fiber.Ctx) error {
	var req logoutRequest
	_ = c.BodyParser(&req)

	if err := h.service.Logout(c.Context(), req.RefreshToken); err != nil {
		return response.InternalError(c, "logout failed")
	}

	return response.OK(c, "logged out", nil)
}

func (h *Handler) Me(c *fiber.Ctx) error {
	claims, err := middleware.ClaimsFromContext(c)
	if err != nil {
		return response.Unauthorized(c, "unauthorized")
	}

	me, err := h.service.Me(c.Context(), claims.UserID, claims.TeamID, claims.Role)
	if err != nil {
		return h.mapError(c, err)
	}

	return response.OK(c, "ok", me)
}

func (h *Handler) mapError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, ErrInvalidCredentials):
		return response.Unauthorized(c, err.Error())
	case errors.Is(err, ErrInvalidRefresh):
		return response.Unauthorized(c, err.Error())
	case errors.Is(err, ErrWeakPassword):
		return response.BadRequest(c, err.Error())
	case errors.Is(err, user.ErrEmailTaken):
		return response.BadRequest(c, err.Error())
	default:
		return response.InternalError(c, "something went wrong")
	}
}

func toSessionResponse(session *Session) fiber.Map {
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
