package auth

import (
	"time"

	"github.com/google/uuid"
)

type RefreshToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TeamID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

type Session struct {
	UserID       uuid.UUID `json:"user_id"`
	TeamID       uuid.UUID `json:"team_id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	TeamName     string    `json:"team_name"`
	TeamSlug     string    `json:"team_slug"`
	Role         string    `json:"role"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
}

type UserProfile struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	Name  string    `json:"name"`
}

type TeamProfile struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Slug string    `json:"slug"`
}

type MeResponse struct {
	User UserProfile `json:"user"`
	Team TeamProfile `json:"team"`
	Role string      `json:"role"`
}
