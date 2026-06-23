package user

import (
	"time"

	"github.com/google/uuid"
)

// User is the workspace user domain model.
type User struct {
	ID           uuid.UUID
	Email        string
	Name         string
	PasswordHash string
	LastActiveAt *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
}

// RefreshToken stores hashed refresh token metadata.
type RefreshToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TeamID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}
