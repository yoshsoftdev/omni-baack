package meta

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type EmbeddedSignupSessionStatus string

const (
	EmbeddedSignupPending   EmbeddedSignupSessionStatus = "pending"
	EmbeddedSignupCompleted EmbeddedSignupSessionStatus = "completed"
	EmbeddedSignupFailed    EmbeddedSignupSessionStatus = "failed"
	EmbeddedSignupExpired   EmbeddedSignupSessionStatus = "expired"
)

const defaultSessionTTL = time.Hour

// EmbeddedSignupSession is a durable Meta embedded signup flow record.
type EmbeddedSignupSession struct {
	ID            uuid.UUID
	TeamID        uuid.UUID
	UserID        *uuid.UUID
	ChannelID     *uuid.UUID
	BusinessID    *string
	Status        EmbeddedSignupSessionStatus
	OAuthState    string
	OAuthCodeHash *string
	ErrorMessage  *string
	CreatedAt     time.Time
	ExpiresAt     time.Time
	CompletedAt   *time.Time
}

// EmbeddedSignupSessionRepository persists embedded signup sessions.
type EmbeddedSignupSessionRepository interface {
	Create(ctx context.Context, session *EmbeddedSignupSession) error
	FindByID(ctx context.Context, teamID, id uuid.UUID) (*EmbeddedSignupSession, error)
	FindByOAuthState(ctx context.Context, teamID uuid.UUID, oauthState string) (*EmbeddedSignupSession, error)
	FindByOAuthCodeHash(ctx context.Context, teamID uuid.UUID, codeHash string) (*EmbeddedSignupSession, error)
	Update(ctx context.Context, session *EmbeddedSignupSession) error
}
