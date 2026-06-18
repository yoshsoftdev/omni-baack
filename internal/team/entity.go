package team

import (
	"time"

	"github.com/google/uuid"
)

type Role string

const (
	RoleOwner      Role = "owner"
	RoleAdmin      Role = "admin"
	RoleSupervisor Role = "supervisor"
	RoleAgent      Role = "agent"
)

type MemberStatus string

const (
	MemberStatusActive   MemberStatus = "active"
	MemberStatusInactive MemberStatus = "inactive"
)

type Team struct {
	ID        uuid.UUID
	Name      string
	Slug      string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

type Member struct {
	ID        uuid.UUID
	TeamID    uuid.UUID
	UserID    uuid.UUID
	RoleID    uuid.UUID
	Role      Role
	Status    MemberStatus
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

type MemberWithUser struct {
	ID           uuid.UUID    `json:"id"`
	UserID       uuid.UUID    `json:"user_id"`
	Email        string       `json:"email"`
	Name         string       `json:"name"`
	Role         Role         `json:"role"`
	RoleID       uuid.UUID    `json:"role_id"`
	Status       MemberStatus `json:"status"`
	LastActiveAt *time.Time   `json:"last_active_at,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
}

type Invitation struct {
	ID        uuid.UUID
	TeamID    uuid.UUID
	Email     string
	Name      string
	RoleID    uuid.UUID
	Role      Role
	Status    string
	InvitedBy uuid.UUID
	CreatedAt time.Time
}

type WorkspaceDetail struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	OwnerName string    `json:"owner_name"`
	OwnerID   uuid.UUID `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
}
